package main

import (
	"bytes"
	"encoding/base64"
	"encoding/csv"
	"encoding/gob"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/gorilla/csrf"
	"github.com/gorilla/mux"
	"github.com/gorilla/schema"
	"github.com/gorilla/sessions"
	"github.com/nicksnyder/go-i18n/v2/i18n"
	sendgrid "github.com/sendgrid/sendgrid-go"
	"github.com/sendgrid/sendgrid-go/helpers/mail"
	qrcode "github.com/skip2/go-qrcode"
	"github.com/stripe/stripe-go"
	stripeClient "github.com/stripe/stripe-go/client"
	"github.com/tintinnabulate/aecontext-handlers/handlers"
	"github.com/tintinnabulate/gonfig"

	"golang.org/x/net/context"
	"golang.org/x/text/language"

	"google.golang.org/appengine/urlfetch"
	guser "google.golang.org/appengine/user"
)

// createHTTPRouter : create a HTTP router where each handler is wrapped by a given context
func createHTTPRouter(f handlers.ToHandlerHOF) *mux.Router {
	appRouter := mux.NewRouter()
	appRouter.HandleFunc("/signup", f(getSignupHandler)).Methods("GET")
	appRouter.HandleFunc("/signup", f(postSignupHandler)).Methods("POST")
	appRouter.HandleFunc("/register", f(getRegistrationFormHandler)).Methods("GET")
	appRouter.HandleFunc("/register", f(postRegistrationFormHandler)).Methods("POST")
	appRouter.HandleFunc("/charge", f(postRegistrationFormPaymentHandler)).Methods("POST")
	appRouter.HandleFunc("/registrations.csv", f(getCSVHandler)).Methods("GET")
	appRouter.HandleFunc("/email_qrcodes", f(emailQRCodes)).Methods("GET")
	return appRouter
}

// Fully populated Mail object
func kitchenSink(recipient qrUser, theQRimages [][]byte) []byte {
	m := mail.NewV3Mail()
	sender_address := "test@example.com"
	sender_name := "Example User"
	e := mail.NewEmail(sender_name, sender_address)
	m.SetFrom(e)

	// Customer Subject line
	// if there's only one QR code
	m.Subject = "[EURYPAA 2019] Your EURYPAA QR code!"
	if len(theQRimages) > 1 {
		// if theres more than one
		m.Subject = "[EURYPAA 2019] Your EURYPAA QR codes!"
	}

	p := mail.NewPersonalization()
	tos := []*mail.Email{
		mail.NewEmail(recipient.theUser.First_Name, recipient.theUser.Email_Address),
	}
	p.AddTos(tos...)

	m.AddPersonalizations(p)

	c := mail.NewContent("text/plain", "some text here")
	m.AddContent(c)

	c = mail.NewContent("text/html", "some html here")
	m.AddContent(c)

	// encode and attach all QR images.
	for i, img := range theQRimages {
		a := mail.NewAttachment()
		a.SetContent(base64.StdEncoding.EncodeToString(img))
		a.SetType("image/png")
		a.SetFilename(fmt.Sprintf("QRCODE_%v-EURYPAA_2019.png", i+1))
		a.SetDisposition("inline")
		a.SetContentID(fmt.Sprintf("QR_CODE_%v", i+1))
		m.AddAttachment(a)
	}

	m.AddCategories("EURYPAA 2019")
	m.AddCategories("QR Codes")

	mailSettings := mail.NewMailSettings()
	bypassListManagement := mail.NewSetting(true)
	mailSettings.SetBypassListManagement(bypassListManagement)
	footerSetting := mail.NewFooterSetting()
	footerSetting.SetText("Footer Text")
	footerSetting.SetEnable(true)
	footerSetting.SetHTML("<html><body>Footer Text</body></html>")
	mailSettings.SetFooter(footerSetting)
	spamCheckSetting := mail.NewSpamCheckSetting()
	spamCheckSetting.SetEnable(true)
	spamCheckSetting.SetSpamThreshold(1)
	spamCheckSetting.SetPostToURL("https://spamcatcher.sendgrid.com")
	mailSettings.SetSpamCheckSettings(spamCheckSetting)
	m.SetMailSettings(mailSettings)

	trackingSettings := mail.NewTrackingSettings()
	clickTrackingSettings := mail.NewClickTrackingSetting()
	clickTrackingSettings.SetEnable(true)
	clickTrackingSettings.SetEnableText(true)
	trackingSettings.SetClickTracking(clickTrackingSettings)
	openTrackingSetting := mail.NewOpenTrackingSetting()
	openTrackingSetting.SetEnable(true)
	openTrackingSetting.SetSubstitutionTag("Optional tag to replace with the open image in the body of the message")
	trackingSettings.SetOpenTracking(openTrackingSetting)
	subscriptionTrackingSetting := mail.NewSubscriptionTrackingSetting()
	subscriptionTrackingSetting.SetEnable(true)
	subscriptionTrackingSetting.SetText("text to insert into the text/plain portion of the message")
	subscriptionTrackingSetting.SetHTML("<html><body>html to insert into the text/html portion of the message</body></html>")
	subscriptionTrackingSetting.SetSubstitutionTag("Optional tag to replace with the open image in the body of the message")
	trackingSettings.SetSubscriptionTracking(subscriptionTrackingSetting)
	m.SetTrackingSettings(trackingSettings)

	replyToEmail := mail.NewEmail("Example User", "test@example.com")
	m.SetReplyTo(replyToEmail)

	return mail.GetRequestBody(m)
}

type qrUser struct {
	theUser     user
	customerIDs []string
}

func emailQRCodes(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	// make sure the person accessing this page is authorised
	u := guser.Current(ctx)
	if u.Email != config.QREmailer {
		http.Error(w, "Invalid User", http.StatusNotFound)
		return
	} else {
		users, err := getAllUsers(ctx)
		if err != nil {
			http.Error(w, fmt.Sprintf("could not get users: %v", err), http.StatusInternalServerError)
			return
		}

		// make a map of email address -> user, because there are multiple payments made by the same email
		email_user := make(map[string]qrUser)

		for _, u := range users {
			email_user[u.Email_Address] = qrUser{
				theUser: u,
				// append customer IDs inside this array
				customerIDs: append(email_user[u.Email_Address].
					customerIDs, u.Stripe_Customer_ID)}
		}

		// make 1 QR image per customer ID
		var pngs [][]byte
		for _, cus := range email_user[config.QREmailer].customerIDs {
			png, _ := qrcode.Encode(cus, qrcode.Medium, 256)
			pngs = append(pngs, png)
		}

		sendgrid.DefaultClient.HTTPClient = urlfetch.Client(ctx)

		request := sendgrid.GetRequest(config.SendGridKey, "/v3/mail/send", "https://api.sendgrid.com")
		request.Method = "POST"
		// attach all the QR codes into a message bound for that email address
		request.Body = kitchenSink(email_user[config.QREmailer], pngs)
		// send the email
		response, err := sendgrid.API(request)
		if err != nil {
			fmt.Fprintf(w, "%v", err)
		} else {
			fmt.Fprintf(w, "%v", response.StatusCode)
			fmt.Fprintf(w, "%v", response.Body)
			fmt.Fprintf(w, "%v", response.Headers)
		}
		return
	}

}

// getCSVHandler : get CSV
func getCSVHandler(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	u := guser.Current(ctx)
	if u.Email != config.CSVUser {
		http.Error(w, "Invalid User", http.StatusNotFound)
		return
	} else {
		b := &bytes.Buffer{}
		c := csv.NewWriter(b)

		if err := c.Write([]string{
			"creation_date",
			"first_name",
			"email_address",
			"country",
			"city",
			"is_servant",
			"is_outreacher",
			"wants_tshirt",
			"fellowship",
			"stripe_charge_id",
			"stripe_customer_id",
		}); err != nil {
			log.Fatalln("error writing record to csv:", err)
			return
		}

		users, err := getAllUsers(ctx)
		if err != nil {
			http.Error(w, fmt.Sprintf("could not get users: %v", err), http.StatusInternalServerError)
			return
		}

		for _, u := range users {
			var record []string
			record = append(record, fmt.Sprint(u.Creation_Date))
			record = append(record, u.First_Name)
			record = append(record, u.Email_Address)
			record = append(record, fmt.Sprint(u.Country))
			record = append(record, u.City)
			record = append(record, fmt.Sprint(u.IsServant))
			record = append(record, fmt.Sprint(u.IsOutreacher))
			record = append(record, fmt.Sprint(u.IsTshirtBuyer))
			record = append(record, fmt.Sprint(u.Member_Of))
			record = append(record, u.Stripe_Charge_ID)
			record = append(record, u.Stripe_Customer_ID)
			if err := c.Write(record); err != nil {
				log.Fatalln("error writing record to csv:", err)
			}
		}

		c.Flush()

		if err := c.Error(); err != nil {
			log.Fatal(err)
			return
		}
		w.Header().Set("Content-Disposition", "attachment; filename=registrations.csv")
		w.Header().Set("Content-Type", r.Header.Get("Content-Type"))
		w.Header().Set("Content-Description", "File Transfer")
		io.Copy(w, b)
		return
	}
}

// getSignupHandler : show the signup form (SignupURL)
func getSignupHandler(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	convention, err := getLatestConvention(ctx)
	if err != nil {
		http.Error(w, fmt.Sprintf("could not get latest convention: %v", err), http.StatusInternalServerError)
		return
	}
	tmpl := templates.Lookup("signup_form.tmpl")
	page := &pageInfo{convention: convention, localizer: getLocalizer(r), r: r}
	tmpl.Execute(w, getVars(page))
}

// postSignupHandler : use the signup service to send the person a verification URL
func postSignupHandler(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	convention, err := getLatestConvention(ctx)
	if err != nil {
		http.Error(w, fmt.Sprintf("could not get latest convention: %v", err), http.StatusInternalServerError)
		return
	}
	err = r.ParseForm()
	if err != nil {
		http.Error(w, fmt.Sprintf("could not parse email form: %v", err), http.StatusInternalServerError)
		return
	}
	var s signup
	err = schemaDecoder.Decode(&s, r.PostForm)
	if err != nil {
		http.Error(w, fmt.Sprintf("could not encode email address: %v", err), http.StatusInternalServerError)
		return
	}
	httpClient := urlfetch.Client(ctx)
	resp, err := httpClient.Post(fmt.Sprintf("%s/%s", config.SignupServiceURL, s.Email_Address), "", nil)
	if err != nil {
		http.Error(w, fmt.Sprintf("could not connect to email verifier: %v", err), http.StatusInternalServerError)
		return
	}
	if resp.StatusCode != http.StatusOK {
		http.Error(w, "could not send verification email", resp.StatusCode)
		return
	}
	tmpl := templates.Lookup("check_email.tmpl")
	page := &pageInfo{convention: convention, localizer: getLocalizer(r), r: r}
	tmpl.Execute(w, getVars(page))
}

// getRegistrationFormHandler : show the registration form
func getRegistrationFormHandler(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	convention, err := getLatestConvention(ctx)
	if err != nil {
		http.Error(w, fmt.Sprintf("could not get latest convention: %v", err), http.StatusInternalServerError)
		return
	}
	tmpl := templates.Lookup("registration_form.tmpl")
	page := &pageInfo{convention: convention, localizer: getLocalizer(r), r: r}
	tmpl.Execute(w, getVars(page))
}

// postRegistrationFormHandler : if they've signed up, show the payment form, otherwise redirect to SignupURL
func postRegistrationFormHandler(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	var regform registrationForm
	var s signup
	err := r.ParseForm()
	if err != nil {
		http.Error(w, fmt.Sprintf("could not parse registration form: %v", err), http.StatusInternalServerError)
		return
	}
	err = schemaDecoder.Decode(&regform, r.PostForm)
	if err != nil {
		http.Error(w, fmt.Sprintf("could not encode registration form: %v", err), http.StatusInternalServerError)
		return
	}
	httpClient := urlfetch.Client(ctx)
	resp, err := httpClient.Get(fmt.Sprintf("%s/%s", config.SignupServiceURL, regform.Email_Address))
	if err != nil {
		http.Error(w, fmt.Sprintf("could not connect to email verifier: %v", err), http.StatusInternalServerError)
		return
	}
	if resp.StatusCode != http.StatusOK {
		http.Error(w, "could not verify email address", resp.StatusCode)
		return
	}
	json.NewDecoder(resp.Body).Decode(&s)
	session, err := store.Get(r, "regform")
	if err != nil {
		http.Error(w, fmt.Sprintf("could not create cookie session: %v", err), http.StatusInternalServerError)
		return
	}
	if s.Success {
		session.Values["regform"] = regform
		err := session.Save(r, w)
		if err != nil {
			http.Error(w, fmt.Sprintf("could not save cookie session: %v", err), http.StatusInternalServerError)
			return
		}
		showPaymentForm(ctx, w, r, &regform)
	} else {
		http.Redirect(w, r, "/signup", http.StatusNotFound)
		return
	}
}

func showPaymentForm(ctx context.Context, w http.ResponseWriter, r *http.Request, regform *registrationForm) {
	convention, err := getLatestConvention(ctx)
	if err != nil {
		http.Error(w, fmt.Sprintf("could not get latest convention: %v", err), http.StatusInternalServerError)
		return
	}
	tmpl := templates.Lookup("stripe.tmpl")
	page := &pageInfo{convention: convention, email: regform.Email_Address, localizer: getLocalizer(r), r: r}
	tmpl.Execute(w, getVars(page))
}

// postRegistrationFormPaymentHandler : charge the customer, and create a User in the User table
func postRegistrationFormPaymentHandler(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	convention, err := getLatestConvention(ctx)
	if err != nil {
		http.Error(w, fmt.Sprintf("could not get latest convention: %v", err), http.StatusInternalServerError)
		return
	}
	r.ParseForm()

	emailAddress := r.Form.Get("stripeEmail")

	customerParams := &stripe.CustomerParams{Email: stripe.String(emailAddress)}
	customerParams.SetSource(r.Form.Get("stripeToken"))

	httpClient := urlfetch.Client(ctx)
	sc := stripeClient.New(stripe.Key, stripe.NewBackends(httpClient))

	newCustomer, err := sc.Customers.New(customerParams)
	if err != nil {
		http.Error(w, fmt.Sprintf("could not create customer: %v", err), http.StatusInternalServerError)
		return
	}

	chargeParams := &stripe.ChargeParams{
		Amount:      stripe.Int64(int64(convention.Cost)),
		Currency:    stripe.String(convention.Currency_Code),
		Description: stripe.String(fmt.Sprintf("%s Registration", convention.Name)),
		Customer:    stripe.String(newCustomer.ID),
	}
	charge, err := sc.Charges.New(chargeParams)
	if err != nil {
		fmt.Fprintf(w, "Could not process payment: %v", err)
		return
	}
	session, err := store.Get(r, "regform")
	if err != nil {
		http.Error(w, fmt.Sprintf("could not create cookie session: %v", err), http.StatusInternalServerError)
		return
	}
	var regform *registrationForm
	if v, ok := session.Values["regform"].(*registrationForm); !ok {
		http.Error(w, "could not type assert value from cookie", http.StatusInternalServerError)
		return
	} else {
		regform = v
	}
	user := &user{
		First_Name:         regform.First_Name,
		Last_Name:          regform.Last_Name,
		Email_Address:      regform.Email_Address,
		Password:           regform.Password,
		Country:            regform.Country,
		City:               regform.City,
		Sobriety_Date:      regform.Sobriety_Date,
		Member_Of:          regform.Member_Of,
		IsServant:          regform.IsServant == Yes_Willing,
		IsOutreacher:       regform.IsOutreacher == Yes_Help_Outreach,
		IsTshirtBuyer:      regform.IsTshirtBuyer == Yes_T_Shirt_Please,
		Stripe_Customer_ID: charge.Customer.ID,
		Stripe_Charge_ID:   charge.ID}
	_, err = addUser(ctx, user)
	if err != nil {
		http.Error(w, fmt.Sprintf("could not add new user to user table: %v", err), http.StatusInternalServerError)
		return
	}
	tmpl := templates.Lookup("registration_successful.tmpl")
	page := &pageInfo{convention: convention, localizer: getLocalizer(r), r: r}
	tmpl.Execute(w, getVars(page))
}

// Config is our configuration file format
type Config struct {
	SiteName             string `id:"SiteName"             default:"MyDomain"`
	ProjectID            string `id:"ProjectID"            default:"my-appspot-project-id"`
	CSRFKey              string `id:"CSRF_Key"             default:"my-random-32-bytes"`
	IsLiveSite           bool   `id:"IsLiveSite"           default:"false"`
	SignupURL            string `id:"SignupURL"            default:"this-apps-signup-endpoint.com/signup"`
	SignupServiceURL     string `id:"SignupServiceURL"     default:"http://localhost:10000/signup/eury2019"`
	StripePublishableKey string `id:"StripePublishableKey" default:"pk_live_foo"`
	StripeSecretKey      string `id:"StripeSecretKey"      default:"sk_live_foo"`
	StripeTestPK         string `id:"StripeTestPK"         default:"pk_test_UdWbULsYzTqKOob0SHEsTNN2"`
	StripeTestSK         string `id:"StripeTestSK"         default:"rk_test_xR1MFQcmds6aXvoDRKDD3HdR"`
	TestEmailAddress     string `id:"TestEmailAddress"     default:"foo@example.com"`
	CookieStoreAuth      string `id:"CookieStoreAuth"      default:"xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"`
	CookieStoreEnc       string `id:"CookieStoreEnc"       default:"xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"`
	CSVUser              string `id:"CSVUser"              default:"CSVUser"`
	QREmailer            string `id:"QREmailer"            default:"QREmailer"`
	SendGridKey          string `id:"SendGridKey"          default:"SendGridKey"`
}

var (
	schemaDecoder  *schema.Decoder
	publishableKey string
	templates      *template.Template
	config         Config
	store          *sessions.CookieStore
	translator     *i18n.Bundle
)

func translatorInit() {
	translator = &i18n.Bundle{DefaultLanguage: language.English}
	translator.RegisterUnmarshalFunc("toml", toml.Unmarshal)
	translator.MustLoadMessageFile("locales/active.es.toml")
}

func getLocalizer(r *http.Request) *i18n.Localizer {
	lang := r.FormValue("lang")
	accept := r.Header.Get("Accept-Language")
	return i18n.NewLocalizer(translator, lang, accept)
}

func configInit(configName string) {
	err := gonfig.Load(&config, gonfig.Conf{
		FileDefaultFilename: configName,
		FileDecoder:         gonfig.DecoderJSON,
		FlagDisable:         true,
	})
	if err != nil {
		log.Fatalf("could not load configuration file: %v", err)
		return
	}
	gob.Register(&registrationForm{})
	store = sessions.NewCookieStore(
		[]byte(config.CookieStoreAuth),
		[]byte(config.CookieStoreEnc))
}

// schemaDecoderInit : create the schema decoder for decoding req.PostForm
func schemaDecoderInit() {
	schemaDecoder = schema.NewDecoder()
	schemaDecoder.RegisterConverter(time.Time{}, timeConverter)
	schemaDecoder.IgnoreUnknownKeys(true)
}

// routerInit : initialise our CSRF protected HTTPRouter
func routerInit() {
	// TODO: https://youtu.be/xyDkyFjzFVc?t=1308
	router := createHTTPRouter(handlers.ToHTTPHandler)
	csrfProtector := csrf.Protect(
		[]byte(config.CSRFKey),
		csrf.Secure(config.IsLiveSite))
	csrfProtectedRouter := csrfProtector(router)
	http.Handle("/", csrfProtectedRouter)
}

// stripeInit : set up important Stripe variables
func stripeInit() {
	if config.IsLiveSite {
		publishableKey = config.StripePublishableKey
		stripe.Key = config.StripeSecretKey
	} else {
		publishableKey = config.StripeTestPK
		stripe.Key = config.StripeTestSK
	}
}

// templatesInit : parse the HTML templates, including any predefined functions (FuncMap)
func templatesInit() {
	templates = template.Must(template.New("").
		Funcs(funcMap).
		ParseGlob("templates/*.tmpl"))
}

func init() {
	configInit("config.json")
	templatesInit()
	schemaDecoderInit()
	translatorInit()
	routerInit()
	stripeInit()
}
