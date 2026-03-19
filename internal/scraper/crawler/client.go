package crawler

import (
	"crypto/tls"
	"net/http"
	"time"
)

var crawlerClient = &http.Client{
	Timeout: 10 * time.Second,
}
var robotsClient = &http.Client{
	Timeout: 10 * time.Second,
	Transport: &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
	},
}
