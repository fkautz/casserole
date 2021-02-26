//go:generate bash -c "cd $(mktemp -d) && GO111MODULE=on go get github.com/edwarnicke/imports-gen@v1.1.2"
//go:generate bash -c "GOOS=linux ${GOPATH}/bin/imports-gen"

package imports
