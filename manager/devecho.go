package manager

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"sync"

	"github.com/bushubdegefu/blue-proxy.com/configs"
	"github.com/bushubdegefu/blue-proxy.com/helper"
	"github.com/bushubdegefu/blue-proxy.com/observe"
	"github.com/gorilla/websocket" // Needed for WebSocket support
	"github.com/labstack/echo-contrib/echoprometheus"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/labstack/gommon/log"
	"github.com/spf13/cobra"
	"go.opentelemetry.io/otel/attribute"
)

var (
	env        string
	proxy_otel string
	proxy_tls  string
	devechocli = &cobra.Command{
		Use:   "run",
		Short: "Run Simply blue proxy server",
		Long:  "Run Simply blue proxy server",
		Run: func(cmd *cobra.Command, args []string) {
			if env == "" {
				env = "dev"
			}
			echo_run(env)
		},
	}
)

func otelechospanstarter(next echo.HandlerFunc) echo.HandlerFunc {
	return func(ctx echo.Context) error {
		routeName := ctx.Path() + "_" + strings.ToLower(ctx.Request().Method)
		tracer, span := observe.EchoAppSpanner(ctx, fmt.Sprintf("%v-root", routeName))
		ctx.Set("tracer", &observe.RouteTracer{Tracer: tracer, Span: span})

		err := next(ctx)
		if err != nil {
			return err
		}

		span.SetAttributes(attribute.String("response", fmt.Sprintf("%d", ctx.Response().Status)))
		span.End()
		return nil
	}
}

func echo_run(env string) {
	helper.LoadData()
	configs.AppConfig.SetEnv(env)

	// Initialize OpenTelemetry Tracer if needed
	if proxy_otel == "on" {
		tp := observe.InitTracer()
		defer tp.Shutdown(context.Background())
	}

	app := echo.New()

	// Middleware stack
	app.Use(middleware.CORS())
	app.Use(echoprometheus.NewMiddleware("echo_blue"))
	app.Use(middleware.RateLimiter(middleware.NewRateLimiterMemoryStore(1000)))
	app.Use(middleware.RecoverWithConfig(middleware.RecoverConfig{
		StackSize: 1 << 10,
		LogLevel:  log.ERROR,
	}))

	// OpenTelemetry middleware if needed
	if proxy_otel == "on" {
		app.Use(otelechospanstarter)
	}

	// Setup Proxy
	targets, err := getting_URL()
	if err != nil {
		panic(err)
	}

	// Load balancing setup
	loadBalancer := &RoundRobinBalancer{
		Targets: targets,
	}

	// Setup the proxy handler for each request
	app.Any("/*", func(c echo.Context) error {
		// Use the load balancer to get the next target to forward the request to
		target := loadBalancer.NextTarget()

		// Check if it's a WebSocket request and upgrade if necessary
		if isWebSocketUpgrade(c.Request()) {
			return handleWebSocketUpgrade(c, target.URL)
		}

		// Handle static files and other normal requests
		return forwardRequestToTarget(c, target.URL)
	})

	// Start the server
	go startServer(app)

	// Graceful shutdown
	waitForShutdown()
}

func isWebSocketUpgrade(req *http.Request) bool {
	return req.Header.Get("Upgrade") == "websocket" && req.Header.Get("Connection") == "Upgrade"
}

func handleWebSocketUpgrade(c echo.Context, targetURL *url.URL) error {
	// Upgrade the incoming HTTP request to a WebSocket
	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true // Allow all origins (you can adjust this for security)
		},
	}
	conn, err := upgrader.Upgrade(c.Response(), c.Request(), nil)
	if err != nil {
		return fmt.Errorf("failed to upgrade to WebSocket: %w", err)
	}
	defer conn.Close()

	// Create a new WebSocket request to the target server
	req, err := http.NewRequest(c.Request().Method, targetURL.String()+c.Request().RequestURI, c.Request().Body)
	if err != nil {
		return fmt.Errorf("failed to create WebSocket request: %w", err)
	}

	// Copy headers from the incoming request to the new request
	for key, values := range c.Request().Header {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}

	// Create an HTTP client for WebSocket handling
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				ServerName:         targetURL.Hostname(),
				InsecureSkipVerify: true,
			},
		},
	}

	// Send the request to the target server (this is a WebSocket upgrade request)
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send WebSocket request: %w", err)
	}
	defer resp.Body.Close()

	// Establish WebSocket connection with the target server
	targetConn, _, err := websocket.DefaultDialer.Dial(resp.Request.URL.String(), nil)
	if err != nil {
		return fmt.Errorf("failed to establish WebSocket connection to target: %w", err)
	}
	defer targetConn.Close()

	// Forward messages between the client WebSocket and target WebSocket
	go forwardWebSocketMessages(conn, targetConn)
	return nil
}

func forwardWebSocketMessages(clientConn, targetConn *websocket.Conn) {
	// Forward messages from client to target and vice versa
	for {
		msgType, msg, err := clientConn.ReadMessage()
		if err != nil {
			break
		}
		err = targetConn.WriteMessage(msgType, msg)
		if err != nil {
			break
		}
	}
}

func forwardRequestToTarget(c echo.Context, targetURL *url.URL) error {

	target_url := fmt.Sprintf("%v%v", targetURL.String(), c.Request().RequestURI)
	req, err := http.NewRequest(c.Request().Method, target_url, c.Request().Body)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Copy headers from the incoming request to the new request
	for key, values := range c.Request().Header {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}

	// Create an HTTP client
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				ServerName:         targetURL.Hostname(),
				InsecureSkipVerify: true,
			},
		},
		// Allow following redirects
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			// Optionally, limit the number of redirects
			if len(via) >= 10 {
				return fmt.Errorf("too many redirects")
			}
			// Print the redirect URL to debug
			fmt.Println("Redirected to:", req.URL)
			return nil // Follow the redirect
		},
	}

	// Send the request to the target server
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request to target: %w", err)
	}
	defer resp.Body.Close()

	// Copy the response headers and status code
	for key, values := range resp.Header {
		for _, value := range values {
			c.Response().Header().Add(key, value)
		}
	}

	c.Response().WriteHeader(resp.StatusCode)

	// Read the response body into a byte slice
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	// Copy the response body to the client
	_, err = c.Response().Write(body)
	if err != nil {
		return fmt.Errorf("failed to write response body: %w", err)
	}

	return nil
}

func startServer(app *echo.Echo) {
	HTTP_PORT := configs.AppConfig.Get("HTTP_PORT")
	if proxy_tls == "on" {
		CERT_FILE := "./server.pem"
		KEY_FILE := "./server-key.pem"
		app.Logger.Fatal(app.StartTLS("0.0.0.0:"+HTTP_PORT, CERT_FILE, KEY_FILE))
	} else {
		app.Logger.Fatal(app.Start("0.0.0.0:" + HTTP_PORT))
	}
}

func waitForShutdown() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	<-c
	fmt.Println("Gracefully shutting down...")
}

func getting_URL() ([]*middleware.ProxyTarget, error) {
	helper.LoadData()
	var urls []*middleware.ProxyTarget
	for _, value := range helper.Targets.Targets {
		url, err := url.Parse(value)
		if err != nil {
			return nil, err
		}
		if url.Scheme != "http" && url.Scheme != "https" {
			return nil, fmt.Errorf("invalid target URL scheme: %s", url.Scheme)
		}
		urls = append(urls, &middleware.ProxyTarget{URL: url})
	}
	return urls, nil
}

// RoundRobinBalancer is a simple load balancer
type RoundRobinBalancer struct {
	Targets []*middleware.ProxyTarget
	mu      sync.Mutex
	index   int
}

// NextTarget returns the next target in round-robin fashion
func (r *RoundRobinBalancer) NextTarget() *middleware.ProxyTarget {
	r.mu.Lock()
	defer r.mu.Unlock()
	target := r.Targets[r.index]
	r.index = (r.index + 1) % len(r.Targets) // Round-robin logic
	return target
}

func init() {
	devechocli.Flags().StringVar(&env, "env", "help", "Which environment to run for example prod or dev")
	devechocli.Flags().StringVar(&proxy_otel, "otel", "help", "Turn on/off OpenTelemetry tracing")
	devechocli.Flags().StringVar(&proxy_tls, "tls", "help", "Turn on/off tls, \"on\" for auto on and \"off\" for auto off")
	goFrame.AddCommand(devechocli)
}
