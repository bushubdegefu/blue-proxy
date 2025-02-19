package manager

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"time"

	"net/http/httptrace"

	"github.com/bushubdegefu/blue-proxy/configs"
	"github.com/bushubdegefu/blue-proxy/helper"
	"github.com/bushubdegefu/blue-proxy/logger"
	"github.com/bushubdegefu/blue-proxy/observe"
	"github.com/gorilla/websocket" // Needed for WebSocket support
	"github.com/labstack/echo-contrib/echoprometheus"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/labstack/gommon/log"
	"github.com/madflojo/tasks"
	"github.com/spf13/cobra"
	"go.opentelemetry.io/contrib/instrumentation/net/http/httptrace/otelhttptrace"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/time/rate"
)

var (
	env           string
	proxy_otel    string
	proxy_tls     string
	proxy_logging string
	devechocli    = &cobra.Command{
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

	// adding file logger
	logOutput := os.Stdout

	if proxy_logging == "on" {

		logOutput, _ = logger.Logfile()
	}

	app.Use(middleware.LoggerWithConfig(middleware.LoggerConfig{
		Format: `{"time":"${time_rfc3339_nano}","id":"${id}","remote_ip":"${remote_ip}",` +
			`"host":"${host}","method":"${method}","uri":"${uri}","user_agent":"${user_agent}",` +
			`"status":${status},"error":"${error}","latency":${latency},"latency_human":"${latency_human}"` +
			`,"bytes_in":${bytes_in},"bytes_out":${bytes_out}}` + "\n",
		Output: logOutput,
	}))

	// Middleware stack
	configLimit, _ := strconv.ParseFloat(configs.AppConfig.GetOrDefault("RATE_LIMIT_PER_SECOND", "50000"), 64)
	rateLimit := rate.Limit(configLimit)

	//  cross orign allow middleware
	app.Use(middleware.CORS())

	//  rate limit middleware
	app.Use(middleware.RateLimiter(middleware.NewRateLimiterMemoryStore(rateLimit)))

	//  prometheus middleware
	app.Use(echoprometheus.NewMiddleware("blue_proxy_v_0"))

	// Recover middleware
	app.Use(middleware.RecoverWithConfig(middleware.RecoverConfig{
		StackSize: 1 << 10,
		LogLevel:  log.ERROR,
	}))

	// OpenTelemetry middleware if needed
	if proxy_otel == "on" {
		app.Use(otelechospanstarter)
	}

	// Setup Proxy targets
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

	// getting log clearing task
	log_truncate := logger.ScheduledTasks()

	// Start the server
	go startServer(app)

	// Graceful shutdown
	waitForShutdown(app, log_truncate)
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

// CreateHTTPClientWithOTELAndTLS creates an HTTP client with both OpenTelemetry tracing and TLS configuration.
func createHTTPClientWithOTELAndTLS(targetURL *url.URL, ctx context.Context) *http.Client {
	// Create the base HTTP transport with TLS support
	baseTransport := &http.Transport{
		TLSClientConfig: &tls.Config{
			ServerName:         targetURL.Hostname(),
			InsecureSkipVerify: true, // Set to false in production for security
		},
	}

	// Create a client transport that adds OTEL instrumentation
	otelTransport := otelhttp.NewTransport(
		baseTransport,
		// Add client trace for OpenTelemetry
		otelhttp.WithClientTrace(func(ctx context.Context) *httptrace.ClientTrace {
			// We can return the client trace that will handle the span trace
			return otelhttptrace.NewClientTrace(ctx)
		}),
	)

	// Return the HTTP client with both OTEL and TLS functionality
	return &http.Client{
		Transport: otelTransport,
		// Optional: Add redirect logic (to prevent too many redirects)
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("too many redirects")
			}
			// Log or handle redirect logic here if needed
			fmt.Println("Redirected to:", req.URL)
			return nil // Follow the redirect
		},
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

	// Ensure OpenTelemetry tracing is enabled if otel is on on the client
	if proxy_otel == "on" {
		// Get the tracer from the context
		tracer := c.Get("tracer").(*observe.RouteTracer)
		ctx := tracer.Tracer

		// Start a new span for the outgoing request
		_, span := observe.AppTracer.Start(ctx, fmt.Sprintf("started-proxy-span-%v", rand.Intn(1000)))
		defer span.End() // Ensure the span ends when the function finishes

		// Inject the span context into the outgoing request context
		ctx = trace.ContextWithSpan(ctx, span)

		// Create the HTTP client with OTEL and TLS configuration
		client = createHTTPClientWithOTELAndTLS(targetURL, ctx)
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

// waitForShutdown listens for an interrupt signal (such as SIGINT) and gracefully shuts down the Echo app.
func waitForShutdown(app *echo.Echo, log_truncate *tasks.Scheduler) {
	// Create a context that listens for interrupt signals (e.g., Ctrl+C).
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	// Ensure the stop function is called when the function exits to clean up resources.
	defer stop()

	// Block and wait for an interrupt signal (this will block until the signal is received).
	<-ctx.Done()

	// Once the interrupt signal is received, create a new context with a 10-second timeout.
	// This will allow time for any active requests to complete before forcing shutdown.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel() // Ensure the cancel function is called when the context is no longer needed.

	// Attempt to gracefully shut down the Echo server.
	// If an error occurs during the shutdown process, log the fatal error.
	if err := app.Shutdown(ctx); err != nil {
		app.Logger.Fatal(err)
	}

	// Truncate the log file after shutdown.
	log_truncate.Stop()

	// Log a message indicating the server is being shut down gracefully.
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
	devechocli.Flags().StringVar(&proxy_logging, "logging", "help", "Turn on/off output to file, \"on\" for outputiing log to file")
	goFrame.AddCommand(devechocli)
}
