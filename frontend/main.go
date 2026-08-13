package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"math/rand"
	"net/http"
	"time"
)

var BACKEND_DNS = getEnv("BACKEND_DNS", "localhost")
var BACKEND_PORT = getEnv("BACKEND_PORT", "9000")

// backendBaseURL is where the frontend sends its API calls. It's a
// package-level *variable* (not hardcoded inline) specifically so that
// tests can point it at a fake/mock backend instead of a real one.
var backendBaseURL = fmt.Sprintf("http://%s:%s", BACKEND_DNS, BACKEND_PORT)

type fortune struct {
	ID      string `json:"id" redis:"id"`
	Message string `json:"message" redis:"message"`
}

type newFortune struct {
	Message string `json:"message"`
}

// use a custom client, because we don't do blocking operations wihout timeouts
var myClient = &http.Client{Timeout: 10 * time.Second}

func HealthzHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	io.WriteString(w, "healthy")
}

// RandomHandler asks the backend for one random fortune and writes just
// its message back to the caller.
func RandomHandler(w http.ResponseWriter, r *http.Request) {
	resp, err := myClient.Get(backendBaseURL + "/fortunes/random")
	if err != nil {
		log.Println("backend request failed:", err)
		http.Error(w, "backend unavailable", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	f := new(fortune)
	if err := json.NewDecoder(resp.Body).Decode(f); err != nil {
		log.Println("failed to decode backend response:", err)
		http.Error(w, "bad response from backend", http.StatusBadGateway)
		return
	}

	fmt.Fprint(w, f.Message)
}

// AllHandler asks the backend for every fortune and renders them into
// the fortunes.html template.
func AllHandler(w http.ResponseWriter, r *http.Request) {
	resp, err := myClient.Get(backendBaseURL + "/fortunes")
	if err != nil {
		log.Println("backend request failed:", err)
		http.Error(w, "backend unavailable", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	fortunes := new([]fortune)
	if err := json.NewDecoder(resp.Body).Decode(fortunes); err != nil {
		log.Println("failed to decode backend response:", err)
		http.Error(w, "bad response from backend", http.StatusBadGateway)
		return
	}

	tmpl, err := template.ParseFiles("./templates/fortunes.html")
	if err != nil {
		log.Println("failed to parse template:", err)
		http.Error(w, "template error", http.StatusInternalServerError)
		return
	}

	tmpl.Execute(w, fortunes)
}

// AddHandler forwards a new fortune message to the backend to be saved.
func AddHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Use POST", http.StatusMethodNotAllowed)
		return
	}

	f := new(newFortune)
	if err := json.NewDecoder(r.Body).Decode(f); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	postUrl := backendBaseURL + "/fortunes"
	jsonStr := []byte(fmt.Sprintf(`{"id": "%d", "message": "%s"}`, rand.Intn(10000), f.Message))

	_, err := myClient.Post(postUrl, "application/json", bytes.NewBuffer(jsonStr))
	if err != nil {
		log.Println("backend request failed:", err)
		http.Error(w, "backend unavailable", http.StatusBadGateway)
		return
	}

	fmt.Fprint(w, "Cookie added!")
}

func main() {
	http.HandleFunc("/healthz", HealthzHandler)
	http.HandleFunc("/api/random", RandomHandler)
	http.HandleFunc("/api/all", AllHandler)
	http.HandleFunc("/api/add", AddHandler)

	http.Handle("/", http.FileServer(http.Dir("./static")))
	err := http.ListenAndServe(":8080", nil)
	fmt.Printf("%v", err)
}
