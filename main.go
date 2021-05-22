package main

import (
	"context"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/spf13/viper"
)

type handler struct{}

var (
	webhookProxy  *WebhookProxy
	webhookStatus int // e.g. 200
	webhookMethod string
)

func initialize() {
	rand.Seed(time.Now().UnixNano())

	log.Println("Reading config...")

	path, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}

	viper.AddConfigPath(path)
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err != nil {
		log.Fatal(err)
	}

	webhookStatus = viper.GetInt("webhook.return")
	webhookMethod = viper.GetString("webhook.method")

	remoteUrl, err := url.Parse(viper.GetString("proxy.remote_url"))
	if err != nil {
		log.Fatal(err)
	}

	webhookProxy, err = NewWebhookProxy(
		&WebhookProxyConfig{
			Method:         viper.GetString("webhook.method"),
			RemoteHost:     remoteUrl.Host,
			RemoteScheme:   remoteUrl.Scheme,
			ContentType:    viper.GetString("webhook.content_type"),
			NumClients:     viper.GetInt("proxy.num_clients"),
			RequestTimeout: time.Duration(viper.GetInt("proxy.request_timeout")),
		},
	)
	if err != nil {
		log.Fatal(err)
	}
}

func main() {
	initialize()

	log.Println("Starting server...")

	srv := &http.Server{
		Addr:         viper.GetString("server.bind"),
		Handler:      handler{},
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
	}

	srv.SetKeepAlivesEnabled(false)

	log.Fatal(srv.ListenAndServe())
}

func generateRequestId() int {
	return rand.Intn(10_000_000)
}

func (h handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	requestId := generateRequestId()
	r = r.WithContext(context.WithValue(r.Context(), "reqid", requestId))

	log.Printf("reqid=%d, <- %s %s (%s)", requestId, r.Method, r.RequestURI, r.RemoteAddr)
	if r.Method == webhookMethod {
		go webhookProxy.HandleRequest(r)
		w.WriteHeader(webhookStatus)
	} else {
		w.WriteHeader(http.StatusNotFound)
	}
}
