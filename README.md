# BlueProxy

**BlueProxy** is a simple and flexible reverse proxy server built using the [Echo](https://echo.labstack.com/) web framework in Go. It allows you to forward incoming HTTP and WebSocket requests to multiple target servers with support for load balancing, static file serving, WebSocket handling, and SSL/TLS encryption. Additionally, BlueProxy can be integrated with [OpenTelemetry](https://opentelemetry.io/) for distributed tracing and observability.

---

## Features

- **Reverse Proxy**: Forwards requests to multiple backend servers with support for load balancing.
- **Load Balancing**: Implements a round-robin load balancing strategy to distribute requests evenly across target servers.
- **WebSocket Support**: Handles WebSocket connections with upgrade handling and forwards WebSocket traffic to target servers.
- **Static File Proxying**: Supports proxying static files such as images, CSS, and JavaScript to target servers.
- **SSL/TLS Support**: Automatically enables TLS support with a custom certificate and key for secure communication.
- **OpenTelemetry Integration**: (Optional) Allows integration with OpenTelemetry for distributed tracing and observability.
- **Customizable Headers**: Forwards all request headers to the target server with optional modifications.

---

## Installation

1. **Install BlueProxy**:

    ```bash
    go install github.com/bushubdegefu/blue-proxy@latest
    ```

2. **Create Project Directory**:

    ```bash
    mkdir my-new-project && cd my-new-project
    ```

3. **Generate `target.json` and Configuration Files**:

    BlueProxy requires `target.json` and configuration environment files to run.

    ```bash
    blue-proxy targets
    blue-proxy env
    ```

    - Update the `target.json` file with your backend server URLs.
    - Update the `.dev.env` file inside the `config` folder with your environment variables.

4. **Run the Server**:

    BlueProxy requires some Go modules. You can install them using the following command:

    ```bash
    blue-proxy run --env=dev --tls=on  # Default TLS is off
    ```

    If you want to load config from a `.prod` file, replace "dev" with "prod" in the above command.

    To enable OpenTelemetry (OTel) tracing, use the `otel` flag (default is off):
    ### Note:
    If you enable TLS, you need to provide a certificate and key file. These files should be named `server.pem` (certificate) and `server-key.pem` (private key).


    ```bash
    blue-proxy run --env=dev --tls=on --otel=on  # Default TLS is off
    ```
    Alternatively, you can clone the repository and compile BlueProxy locally:

    ```bash
    git clone https://github.com/bushubdegefu/blue-proxy.git
    cd blue-proxy
    go build -tags netgo -ldflags '-s -w' -o blue-proxy
    ```

---

## Configuration

BlueProxy relies on the following configuration files and environment variables:

### `target.json`

This file contains a list of target backend servers to which BlueProxy will forward requests. Each target should be a valid URL.

#### Example `.dev.env` File:
```env
APP_NAME=dev
HTTP_PORT=8700
TEST_NAME="Development Environment"
BODY_LIMIT=70
READ_BUFFER_SIZE=40
RATE_LIMIT_PER_SECOND=5000

# Observability settings
TRACE_EXPORTER=jaeger
TRACER_HOST=localhost
TRACER_PORT=14317

TARGET_HOST_NAME=somedomain.com
```

#### Example `target.json`:

```json
{
  "targets": [
    "http://serviceA.com",
    "http://serviceB.com"
  ]
}

```

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

### MIT License
