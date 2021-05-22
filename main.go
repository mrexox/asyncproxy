package main

import (
	"context"
	"log"
	"math/rand"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/spf13/viper"
)

var (
	webhookProxy *WebhookProxy
)

func init() {
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

	webhookProxy, err = NewWebhookProxy(
		&WebhookProxyConfig{
			url:            viper.GetString("proxy.notifications_url"),
			numClients:     viper.GetInt("proxy.num_clients"),
			requestTimeout: time.Duration(viper.GetInt("proxy.request_timeout")),
		},
	)
	if err != nil {
		log.Fatal(err)
	}
}

func main() {
	log.Println("Starting server...")

	bind := viper.GetString("server.bind")
	endpoint := viper.GetString("server.endpoint")

	srv := &http.Server{
		Addr:         bind,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
	}

	srv.SetKeepAlivesEnabled(false)

	http.HandleFunc(endpoint, notifications)

	log.Fatal(srv.ListenAndServe())
}

func generateRequestId() int {
	return rand.Intn(10_000_000)
}

func notifications(w http.ResponseWriter, r *http.Request) {
	requestId := generateRequestId()
	r = r.WithContext(context.WithValue(r.Context(), "reqid", requestId))

	log.Printf("reqid=%d, %s %s (%s)", requestId, r.Method, r.RequestURI, r.RemoteAddr)

	go webhookProxy.HandleRequest(r)

	w.WriteHeader(http.StatusOK)
}
