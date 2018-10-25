// Command example runs a sample webserver that uses go-i18n/v2/i18n.
package main

import (
	"fmt"
	"html/template"
	"log"
	"net/http"

	"github.com/BurntSushi/toml"
	"github.com/nicksnyder/go-i18n/v2/i18n"
	"golang.org/x/text/language"
)

var page = template.Must(template.New("").Parse(`
<!DOCTYPE html>
<html>
<body>

<h1>{{.Title}}</h1>

{{range .Paragraphs}}<p>{{.}}</p>{{end}}

</body>
</html>
`))

func main() {
	bundle := &i18n.Bundle{DefaultLanguage: language.English}
	bundle.RegisterUnmarshalFunc("toml", toml.Unmarshal)
	// No need to load active.en.toml since we are providing default translations.
	// bundle.MustLoadMessageFile("active.en.toml")
	bundle.MustLoadMessageFile("active.es.toml")

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		lang := r.FormValue("lang")
		accept := r.Header.Get("Accept-Language")
		localizer := i18n.NewLocalizer(bundle, lang, accept)

		name := "Bob"

		btnCompletePayment := localizer.MustLocalize(&i18n.LocalizeConfig{
			DefaultMessage: &i18n.Message{
				ID:    "btnCompletePayment",
				Other: "Complete payment to Register",
			},
		})

		btnSendVerifEmail := localizer.MustLocalize(&i18n.LocalizeConfig{
			DefaultMessage: &i18n.Message{
				ID:    "btnSendVerifEmail",
				Other: "Send verification email",
			},
		})
		btnContinueToCheckout := localizer.MustLocalize(&i18n.LocalizeConfig{
			DefaultMessage: &i18n.Message{
				ID:    "btnContinueToCheckout",
				Other: "Continue to checkout",
			},
		})
		errProcessPayment := localizer.MustLocalize(&i18n.LocalizeConfig{
			DefaultMessage: &i18n.Message{
				ID:    "errProcessPayment",
				Other: "Could not process payment",
			},
		})
		frmAmount := localizer.MustLocalize(&i18n.LocalizeConfig{
			DefaultMessage: &i18n.Message{
				ID:    "frmAmount",
				Other: "Amount {{ .CostPrint }} {{ .Currency }}",
			},
			TemplateData: map[string]string{
				"CostPrint": "20",
				"Currency":  "EUR",
			},
		})
		frmCity := localizer.MustLocalize(&i18n.LocalizeConfig{
			DefaultMessage: &i18n.Message{
				ID:    "frmCity",
				Other: "City",
			},
		})
		frmCountry := localizer.MustLocalize(&i18n.LocalizeConfig{
			DefaultMessage: &i18n.Message{
				ID:    "frmCountry",
				Other: "Country",
			},
		})
		frmEnterEmail := localizer.MustLocalize(&i18n.LocalizeConfig{
			DefaultMessage: &i18n.Message{
				ID:    "frmEnterEmail",
				Other: "Please enter your email address",
			},
		})
		frmFirstName := localizer.MustLocalize(&i18n.LocalizeConfig{
			DefaultMessage: &i18n.Message{
				ID:    "frmFirstName",
				Other: "First name",
			},
		})
		frmILiveIn := localizer.MustLocalize(&i18n.LocalizeConfig{
			DefaultMessage: &i18n.Message{
				ID:    "frmILiveIn",
				Other: "I live in...",
			},
		})
		frmPaymentDetails := localizer.MustLocalize(&i18n.LocalizeConfig{
			DefaultMessage: &i18n.Message{
				ID:    "frmPaymentDetails",
				Other: "Payment Details",
			},
		})
		frmSameEmail := localizer.MustLocalize(&i18n.LocalizeConfig{
			DefaultMessage: &i18n.Message{
				ID:    "frmSameEmail",
				Other: "Email - use the same one you verified with",
			},
		})
		frmYourDetails := localizer.MustLocalize(&i18n.LocalizeConfig{
			DefaultMessage: &i18n.Message{
				ID:    "frmYourDetails",
				Other: "Your details",
			},
		})
		pgCheckEmail := localizer.MustLocalize(&i18n.LocalizeConfig{
			DefaultMessage: &i18n.Message{
				ID:    "pgCheckEmail",
				Other: "Please check your email inbox, and click the link we've sent you",
			},
		})
		pgNowRegistered := localizer.MustLocalize(&i18n.LocalizeConfig{
			DefaultMessage: &i18n.Message{
				ID:    "pgNowRegistered",
				Other: "You are now registered!",
			},
		})
		pgRegisterFor := localizer.MustLocalize(&i18n.LocalizeConfig{
			DefaultMessage: &i18n.Message{
				ID:    "pgRegisterFor",
				Other: "Register for {{ .Name }}",
			},
			TemplateData: map[string]string{
				"Name": "EURYPAA",
			},
		})
		pgRegisteredFor := localizer.MustLocalize(&i18n.LocalizeConfig{
			DefaultMessage: &i18n.Message{
				ID:    "pgRegisteredFor",
				Other: "Registered for {{ .Name }}",
			},
			TemplateData: map[string]string{
				"Name": "EURYPAA",
			},
		})
		valEnterEmail := localizer.MustLocalize(&i18n.LocalizeConfig{
			DefaultMessage: &i18n.Message{
				ID:    "valEnterEmail",
				Other: "Please enter a valid email address so we can send you convention details.",
			},
		})
		valFirstName := localizer.MustLocalize(&i18n.LocalizeConfig{
			DefaultMessage: &i18n.Message{
				ID:    "valFirstName",
				Other: "Valid first name is required.",
			},
		})
		valSameEmail := localizer.MustLocalize(&i18n.LocalizeConfig{
			DefaultMessage: &i18n.Message{
				ID:    "valSameEmail",
				Other: "Please enter a valid email address so we can send you convention details.",
			},
		})

		helloPerson := localizer.MustLocalize(&i18n.LocalizeConfig{
			DefaultMessage: &i18n.Message{
				ID:    "HelloPerson",
				Other: "Hello {{.Name}}",
			},
			TemplateData: map[string]string{
				"Name": name,
			},
		})

		err := page.Execute(w, map[string]interface{}{
			"Title": helloPerson,
			"Paragraphs": []string{
				btnCompletePayment,
				btnContinueToCheckout,
				btnSendVerifEmail,
				errProcessPayment,
				frmAmount,
				frmCity,
				frmCountry,
				frmEnterEmail,
				frmFirstName,
				frmILiveIn,
				frmPaymentDetails,
				frmSameEmail,
				frmYourDetails,
				pgCheckEmail,
				pgNowRegistered,
				pgRegisterFor,
				pgRegisteredFor,
				valEnterEmail,
				valFirstName,
				valSameEmail,
			},
		})
		if err != nil {
			panic(err)
		}
	})

	fmt.Println("Listening on http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
