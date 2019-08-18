all:
	# launch dev version of app on localhost
	go generate
	go run .
test:
	# verbose mode, get code coverage, check for race conditions, on all *_test.go files in this package
	go mod download
	go test -v -cover -race ./...
deploy:
	# deploy to live site, creating a new instance (do this before overwrite!).
	gcloud app deploy --project 000000
overwrite:
	# deploy to live site overwriting version 0 (do this only after you've tested deploy!)
	gcloud app deploy --project 000000 --version 0
