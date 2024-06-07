package main

import (
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"sync"
	"time"

	"github.com/gliderlabs/ssh"
	"github.com/gorilla/mux"
	"github.com/teris-io/shortid"
	gossh "golang.org/x/crypto/ssh"
	"golang.org/x/term"
)

type Session struct {
	session     ssh.Session
	destination string
}

var clients sync.Map

type HTTPHandler struct{}

func (h *HTTPHandler) handleWebhook(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]
	value, ok := clients.Load(id)
	if !ok {
		http.Error(w, "client id not found", http.StatusBadRequest)
		return
	}
	session := value.(Session)
	req, err := http.NewRequestWithContext(r.Context(), r.Method, session.destination, r.Body)
	if err != nil {
		http.Error(w, "failed to create request", http.StatusInternalServerError)
		return
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		http.Error(w, "failed to forward request", http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()
	defer r.Body.Close()
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

func startHTTPServer() error {
	httpPort := ":4000"
	router := mux.NewRouter()
	handler := &HTTPHandler{}

	router.HandleFunc("/{id}", handler.handleWebhook).Methods("POST")
	return http.ListenAndServe(httpPort, router)
}

func startSSHServer() error {
	sshPort := ":2222"
	handler := NewSSHHandler()
	fwhandler := &ssh.ForwardedTCPHandler{}
	server := ssh.Server{
		Addr:    sshPort,
		Handler: handler.handleSSHSession,
		ServerConfigCallback: func(ctx ssh.Context) *gossh.ServerConfig {
			cfg := &gossh.ServerConfig{
				ServerVersion: "SSH-2.0-OpenSSH_9.7",
			}
			cfg.Ciphers = []string{"chacha20-poly1305@openssh.com"}
			return cfg
		},
		PublicKeyHandler: func(ctx ssh.Context, key ssh.PublicKey) bool {
			return true
		},
		LocalPortForwardingCallback: ssh.LocalPortForwardingCallback(func(ctx ssh.Context, dhost string, dport uint32) bool {
			log.Println("Accepted forward", dhost, dport)
			return true
		}),
		ReversePortForwardingCallback: ssh.ReversePortForwardingCallback(func(ctx ssh.Context, host string, port uint32) bool {
			log.Println("Attempted to bind", host, port, "granted")
			return true
		}),
		RequestHandlers: map[string]ssh.RequestHandler{
			"tcpip-forward":        fwhandler.HandleSSHRequest,
			"cancel-tcpip-forward": fwhandler.HandleSSHRequest,
		},
	}
	privateKey, err := os.ReadFile("keys/privateKey")
	if err != nil {
		return fmt.Errorf("failed to read private key: %w", err)
	}
	private, err := gossh.ParsePrivateKey(privateKey)
	if err != nil {
		return fmt.Errorf("failed to parse private key: %w", err)
	}
	server.AddHostKey(private)
	return server.ListenAndServe()
}

func main() {
	go func() {
		log.Println("Starting SSH server on :2222")
		if err := startSSHServer(); err != nil {
			log.Fatalf("SSH server error: %v", err)
		}
	}()
	log.Println("Starting HTTP server on :4000")
	if err := startHTTPServer(); err != nil {
		log.Fatalf("HTTP server error: %v", err)
	}
}

type SSHHandler struct{}

func NewSSHHandler() *SSHHandler {
	return &SSHHandler{}
}

func (h *SSHHandler) handleSSHSession(session ssh.Session) {
	if session.RawCommand() == "tunnel" {
		session.Write([]byte("Tunneling traffic to your endpoint"))
		<-session.Context().Done()
		return
	}

	term := term.NewTerminal(session, "$")
	msg := fmt.Sprintf("%s\n\nWelcome to webhooker!\n\nEnter your webhook destination:\n", banner)
	term.Write([]byte(msg))

	input, err := term.ReadLine()
	if err != nil {
		log.Printf("failed to read input: %v", err)
		return
	}

	destination, err := url.Parse(input)
	if err != nil {
		log.Printf("invalid URL: %v", err)
		return
	}

	generatePort := randomPort()
	internalSession := Session{
		session:     session,
		destination: destination.String(),
	}

	id := shortid.MustGenerate()
	clients.Store(id, internalSession)
	webhookURL := fmt.Sprintf("http://localhost:4000/%s", id)
	command := fmt.Sprintf("\nGenerate webhook: %s\n\nCommand to copy:\nssh -R 127.0.0.1:%d:%s localhost -p 2222 tunnel\n", webhookURL, generatePort, destination.Host)
	session.Write([]byte(command))
}

func randomPort() int {
	rand.Seed(time.Now().UnixNano())
	min := 50000
	max := 65535
	return rand.Intn(max-min+1) + min
}

var banner = ` _  _  _       _     _                 _     
| || || |     | |   | |               | |    
| || || | ____| | _ | | _   ___   ___ | |  _ 
| ||_|| |/ _  ) || \| || \ / _ \ / _ \| | / )
| |___| ( (/ /| |_) ) | | | |_| | |_| | |< ( 
 \______|\____)____/|_| |_|\___/ \___/|_| \_)
                                             `
