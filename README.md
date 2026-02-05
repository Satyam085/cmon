# CMON - Complaint Monitoring System

Automated complaint monitoring and notification system for DGVCL (Dakshin Gujarat Vij Company Limited). Scrapes complaint data from the DGVCL portal and sends real-time notifications via Telegram.

## Features

### Core Functionality
- ✅ **Automated Complaint Monitoring**: Periodic fetching with configurable intervals
- ✅ **Pagination Support**: Handles large datasets across multiple pages
- ✅ **Real-time Telegram Notifications**: Formatted messages with complaint details
- ✅ **Interactive Resolution**: Resolve complaints directly from Telegram with callback buttons
- ✅ **Resolved Complaint Tracking**: Automatically edits messages when complaints are resolved
- ✅ **Automatic Session Management**: Retry logic with session recovery

### Performance & Reliability
- ⚡ **Worker Pool Processing**: Concurrent complaint processing with configurable pool size
- ⚡ **Connection Pooling**: Reusable HTTP client to prevent connection exhaustion
- ⚡ **Batch CSV Writes**: Configurable batch size for efficient storage operations
- 🔄 **Three-tier Retry Logic**: Retry → Re-login → Browser restart on failures
- 🔒 **Thread-safe Storage**: Mutex-protected complaint storage operations

### Developer Experience
- 🛠️ **Embedded Configuration**: Binary works standalone with embedded `.env` file
- 🛠️ **Debug Mode**: Simulates API calls for safe testing without affecting production
- 🛠️ **Health Check Endpoint**: Monitor application status and uptime
- 🛠️ **Cross-platform Builds**: Makefile and GitHub Actions for Windows, Linux, Android
- 📊 **Comprehensive Logging**: Detailed logs with emoji indicators for easy scanning


## Prerequisites

- Go 1.25+
- Chrome/Chromium browser (for ChromeDP)
- Telegram account (for notifications)

## Installation

```bash
git clone <repo>
cd cmon
go build -o cmon .
```

## Configuration

CMON uses a **three-tier configuration system** for maximum flexibility:

1. **Embedded `.env` file** (lowest priority) - Compiled into the binary at build time
2. **External `.env` file** (medium priority) - Loaded from the current directory at runtime
3. **Environment variables** (highest priority) - Set in your shell or deployment environment

This allows the binary to work standalone while still supporting runtime configuration changes.

### Quick Start Configuration

For development, create a `.env` file in the project root:

```env
# DGVCL Portal Credentials (REQUIRED)
DGVCL_USERNAME=your_username
DGVCL_PASSWORD=your_password

# Telegram Configuration (REQUIRED for notifications)
TELEGRAM_BOT_TOKEN=your_bot_token
TELEGRAM_CHAT_ID=your_chat_id

# Portal URLs (usually don't need to change)
LOGIN_URL=https://complaint.dgvcl.com/
COMPLAINT_URL=https://complaint.dgvcl.com/dashboard_complaint_list?from_date=&to_date=&honame=1&coname=21&doname=24&sdoname=87&cStatus=2&commobile=

# Retry Configuration
MAX_LOGIN_RETRIES=3
MAX_FETCH_RETRIES=2
LOGIN_RETRY_DELAY=5s

# Pagination
MAX_PAGES=5

# Timing Configuration
FETCH_INTERVAL=15m
FETCH_TIMEOUT=10m
NAVIGATION_TIMEOUT=60s
WAIT_TIMEOUT=45s

# Performance Tuning
WORKER_POOL_SIZE=10
CACHE_ENABLED=true
BATCH_SIZE=50
HTTP_MAX_CONNS=100
HTTP_TIMEOUT=30s

# Health Check
HEALTH_CHECK_PORT=8080

# Debug Mode (set to true for testing)
DEBUG_MODE=false
```

### Building with Embedded Configuration

The `.env` file in `internal/config/.env` is embedded into the binary during compilation. To update the embedded configuration:

1. Edit `internal/config/.env` with your default values
2. Rebuild the binary: `go build -o cmon .`
3. The binary will now work standalone without requiring an external `.env` file

**Security Note**: Never commit sensitive credentials to `internal/config/.env`. Use template values and override with environment variables in production.

### Environment Variables Reference

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `DGVCL_USERNAME` | Yes | - | DGVCL portal username |
| `DGVCL_PASSWORD` | Yes | - | DGVCL portal password |
| `TELEGRAM_BOT_TOKEN` | Yes | - | Telegram bot API token for notifications |
| `TELEGRAM_CHAT_ID` | Yes | - | Telegram chat ID for notifications |
| `LOGIN_URL` | No | `https://complaint.dgvcl.com/` | Portal login page URL |
| `COMPLAINT_URL` | No | (see config) | Dashboard URL with filters |
| `MAX_LOGIN_RETRIES` | No | 3 | Maximum login attempts before giving up |
| `LOGIN_RETRY_DELAY` | No | 5s | Delay between login retry attempts |
| `MAX_FETCH_RETRIES` | No | 2 | Maximum fetch attempts before alerting |
| `MAX_PAGES` | No | 5 | Maximum pages to fetch per cycle |
| `FETCH_INTERVAL` | No | 15m | How often to check for new complaints |
| `FETCH_TIMEOUT` | No | 10m | Maximum time for entire fetch operation |
| `NAVIGATION_TIMEOUT` | No | 60s | Maximum time for page navigation |
| `WAIT_TIMEOUT` | No | 45s | Maximum time to wait for elements |
| `WORKER_POOL_SIZE` | No | 10 | Number of concurrent workers |
| `CACHE_ENABLED` | No | true | Enable in-memory caching |
| `BATCH_SIZE` | No | 50 | Records to batch before CSV write |
| `HTTP_MAX_CONNS` | No | 100 | Maximum HTTP connections in pool |
| `HTTP_TIMEOUT` | No | 30s | HTTP client timeout |
| `HEALTH_CHECK_PORT` | No | 8080 | Health check server port |
| `DEBUG_MODE` | No | false | Enable debug mode (simulates API calls) |
## Project Structure

The project follows a modular architecture with clear separation of concerns:

```
cmon/
├── main.go                          # Application entry point and orchestration
├── go.mod                           # Go module dependencies
├── go.sum                           # Dependency checksums
├── Makefile                         # Build automation for multiple platforms
├── .env                             # Runtime configuration (not committed)
├── .gitignore                       # Git ignore rules
├── complaints.csv                   # Persistent storage (auto-generated)
│
├── internal/                        # Internal packages (not importable externally)
│   ├── config/                      # Configuration management
│   │   ├── config.go                # Config loading with embedded .env support
│   │   └── .env                     # Embedded configuration (compiled into binary)
│   │
│   ├── auth/                        # Authentication logic
│   │   └── login.go                 # DGVCL portal login with captcha solving
│   │
│   ├── browser/                     # Browser automation
│   │   └── browser.go               # ChromeDP context management
│   │
│   ├── complaint/                   # Complaint processing
│   │   ├── complaint.go             # Main complaint fetching logic
│   │   ├── fetcher.go               # Page scraping and pagination
│   │   └── processor.go             # Worker pool for concurrent processing
│   │
│   ├── storage/                     # Data persistence
│   │   └── storage.go               # CSV-based complaint storage with thread safety
│   │
│   ├── telegram/                    # Telegram integration
│   │   └── telegram.go              # Bot API, message formatting, callback handling
│   │
│   ├── api/                         # External API clients
│   │   ├── http_client.go           # Shared HTTP client with connection pooling
│   │   └── dgvcl.go                 # DGVCL API wrapper
│   │
│   ├── health/                      # Health monitoring
│   │   └── health.go                # HTTP health check endpoint
│   │
│   └── errors/                      # Error handling
│       └── errors.go                # Custom error types and utilities
│
└── .github/                         # GitHub Actions
    └── workflows/
        └── build.yml                # Automated builds for Windows, Linux, Android
```

### Key Architecture Decisions

1. **Embedded Configuration**: The `.env` file in `internal/config/` is embedded at build time, allowing standalone binaries
2. **Worker Pool Pattern**: Concurrent complaint processing with configurable pool size
3. **Separation of Concerns**: Each package has a single, well-defined responsibility
4. **Thread Safety**: Mutex-protected storage and shared state management
5. **Connection Pooling**: Reusable HTTP client to avoid connection exhaustion
6. **Graceful Degradation**: Telegram is optional; app works without it


## Code Execution Flow

```
┌─────────────────────────────────────────────────────────────────┐
│                        main()                                   │
└─────────────────────────────────────────────────────────────────┘
                                │
                                ▼
┌─────────────────────────────────────────────────────────────────┐
│  1. Load Configuration (LoadConfig)                             │
│     - Reads .env file                                           │
│     - Validates required credentials                            │
└─────────────────────────────────────────────────────────────────┘
                                │
                                ▼
┌─────────────────────────────────────────────────────────────────┐
│  2. Initialize Components                                       │
│     - NewComplaintStorage()  → Loads existing complaints.csv    │
│     - NewTelegramConfig()    → Validates bot token/chat ID      │
│     - NewBrowserContext()    → Launches Chrome headless browser │
└─────────────────────────────────────────────────────────────────┘
                                │
                                ▼
┌─────────────────────────────────────────────────────────────────┐
│  3. Login (Login function)                                      │
│     - Navigate to LOGIN_URL                                     │
│     - Fill username/password forms                              │
│     - Submit and wait for dashboard                             │
│     - Retry up to MAX_LOGIN_RETRIES on failure                  │
└─────────────────────────────────────────────────────────────────┘
                                │
                                ▼
┌────────────────────────────────────────────────────────────────┐
│  4. Initial Fetch (FetchComplaints)                            │
│                                                                │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │  for each page (1, 2, 3...)                             │   │
│  │  ├─ fetchComplaintsFromPage()                           │   │
│  │  │  ├─ Navigate to page URL                             │   │
│  │  │  ├─ Extract table data (complaint links)             │   │
│  │  │  ├─ For each new complaint:                          │   │
│  │  │  │  └─ FetchComplaintDetails()                       │   │
│  │  │  │     ├─ Call API endpoint                          │   │
│  │  │  │     ├─ Parse response                             │   │
│  │  │  │     ├─ Send Telegram message                      │   │
│  │  │  │     └─ Return message_id                          │   │
│  │  │  └─ Check pagination (Next button)                   │   │
│  │  └─ Save to complaints.csv (complaint_id, message_id)   │   │
│  └─────────────────────────────────────────────────────────┘   │
│                                                                │
│  5. markResolvedComplaints()                                   │
│     ├─ Get all previously stored complaints                    │
│     ├─ Compare with current website complaints                 │
│     ├─ For resolved (not in current):                          │
│     │  ├─ Edit Telegram message (add "RESOLVED" header)        │
│     │  └─ Remove from complaints.csv                           │
└────────────────────────────────────────────────────────────────┘
                                │
                                ▼
┌────────────────────────────────────────────────────────────────┐
│  6. Refresh Loop (every FETCH_INTERVAL)                        │
│     ├─ fetchWithRetry()                                        │
│     │  ├─ FetchComplaints()                                    │
│     │  ├─ If session expired → re-login → retry                │
│     │  └─ If re-login fails → restart browser → retry          │
│     └─ markResolvedComplaints()                                │
└────────────────────────────────────────────────────────────────┘
```

## Key Functions Reference

| Function | File | Purpose |
|----------|------|---------|
| `main()` | main.go:23 | Entry point, initializes all components and starts the fetch loop |
| `FetchComplaints()` | complaint.go:21 | Main orchestrator for fetching all pages with pagination |
| `fetchComplaintsFromPage()` | complaint.go:57 | Fetches single page, returns complaints + pagination status |
| `FetchComplaintDetails()` | complaint.go:142 | Fetches full complaint details via API, returns Telegram message ID |
| `markResolvedComplaints()` | main.go:254 | Detects resolved complaints, edits Telegram messages, removes from CSV |
| `Login()` | login.go:13 | Handles authentication flow with form submission |
| `fetchWithRetry()` | main.go:142 | Error handling with session recovery and browser restart |
| `EditMessageText()` | telegram.go:206 | Edits existing Telegram message (for resolved status) |
| `NewBrowserContext()` | browser.go:12 | Creates Chrome headless browser context |
| `LoadConfig()` | config.go:37 | Loads and validates configuration from environment |

## Storage Format

```
complaints.csv:
complaint_id,telegram_message_id
12345,123
67890,456
```

The storage maintains:
- All active complaints with their Telegram message IDs
- Automatic cleanup when complaints are resolved
- Thread-safe operations with mutex

## Telegram Message Format

### New Complaint Notification
```
New Complaint
━━━━━━━━━━━━━━━━━━━━

📋 Complaint No: 12345
🔢 Consumer No: 67890

👤 Complainant: John Doe
📱 Mobile: 9876543210
📅 Date: 2026-01-15

━━━━━━━━━━━━━━━━━━━━
Description:
  [Complaint description with preserved formatting]

━━━━━━━━━━━━━━━━━━━━
📍 Location: Main Road
🗺️ Area: City Center
━━━━━━━━━━━━━━━━━━━━
```

### Resolved Complaint Update
```
✅ RESOLVED
━━━━━━━━━━━━━━━━━━━━

Complaint No: 12345
This complaint has been resolved.
━━━━━━━━━━━━━━━━━━━━
```

## Health Check

```
GET http://localhost:8080/health
```

### Response

```json
{
  "status": "healthy",
  "uptime": "1h2m3s",
  "last_fetch_time": "2026-01-15 10:30:00",
  "last_fetch_status": "success"
}
```

## Running the Application

### Development

```bash
# Quick start with go run
go run .

# Build and run
go build -o cmon .
./cmon

# Run in debug mode (simulates API calls, safe for testing)
DEBUG_MODE=true go run .
```

### Production Builds

Using the Makefile for cross-platform builds:

```bash
# Build for current platform
make build

# Build for all platforms (Windows, Linux, Android/Termux)
make build-all

# Build for specific platforms
make build-windows    # Windows 64-bit
make build-linux      # Linux 64-bit
make build-android    # Android/Termux (ARM64)

# Clean build artifacts
make clean
```

Binaries will be created in the `build/` directory:
- `build/cmon-windows-amd64.exe`
- `build/cmon-linux-amd64`
- `build/cmon-android-arm64`

### Deployment

1. **Standalone Binary**: The binary includes embedded configuration and works without external files
2. **With External Config**: Place a `.env` file in the same directory as the binary to override embedded values
3. **Environment Variables**: Set environment variables to override all other configuration sources

Example deployment:

```bash
# Copy binary to server
scp build/cmon-linux-amd64 user@server:/opt/cmon/cmon

# Set environment variables and run
export DGVCL_USERNAME="your_username"
export DGVCL_PASSWORD="your_password"
export TELEGRAM_BOT_TOKEN="your_token"
export TELEGRAM_CHAT_ID="your_chat_id"
./cmon
```


## Common Issues & Solutions

| Issue | Solution |
|-------|----------|
| **Build Error**: `pattern .env: no matching files found` | The `.env` file must exist in `internal/config/` for embedding. Copy `.env` from project root to `internal/config/.env` |
| **Login fails repeatedly** | Check credentials in `.env` file or environment variables, ensure portal is accessible |
| **ChromeDP crashes** | Ensure Chrome/Chromium is installed on the system. On headless servers, install `chromium-browser` |
| **Telegram not sending** | Verify `TELEGRAM_BOT_TOKEN` and `TELEGRAM_CHAT_ID` are correct. Telegram is optional; app works without it |
| **Missing complaints** | Check pagination logic, increase `NAVIGATION_TIMEOUT` and `WAIT_TIMEOUT` values |
| **Session expired errors** | Normal behavior; system automatically re-logs in. No action needed |
| **Browser memory growth** | Consider containerized deployment with periodic restarts, or reduce `FETCH_INTERVAL` |
| **Port 8080 already in use** | Change `HEALTH_CHECK_PORT` to a different port in your configuration |
| **Complaints not resolving** | Ensure `DEBUG_MODE=false` in production. Debug mode simulates API calls without actually resolving |
| **Worker pool errors** | Reduce `WORKER_POOL_SIZE` if experiencing resource constraints |
| **CSV file corruption** | Delete `complaints.csv` and restart. The app will recreate it and refetch all complaints |


## Architecture Highlights

### Design Patterns

1. **Modular Architecture**: Clean separation of concerns with dedicated packages for each responsibility
2. **Worker Pool Pattern**: Concurrent complaint processing with configurable pool size for optimal throughput
3. **Embedded Resources**: Configuration embedded at build time for standalone binary deployment
4. **Three-tier Configuration**: Embedded → External file → Environment variables (increasing precedence)

### Browser & Session Management

5. **Browser Automation**: ChromeDP for headless Chrome automation with automatic captcha solving
6. **Pagination Detection**: DOM-based detection of "Next" button to determine last page
7. **Session Recovery**: Automatic re-login and browser restart on session expiry
8. **Three-tier Error Recovery**: Retry → Re-login → Browser restart for maximum reliability

### Data & Communication

9. **Thread-safe Storage**: Mutex-protected CSV operations for concurrent access
10. **Message Tracking**: Stores Telegram message IDs to enable editing resolved complaints
11. **Connection Pooling**: Reusable HTTP client with configurable connection limits
12. **Batch Processing**: Configurable batch size for efficient CSV writes

### Operations

13. **Health Monitoring**: HTTP endpoint for liveness and readiness checks
14. **Graceful Shutdown**: Handles SIGTERM/SIGINT for clean resource cleanup
15. **Comprehensive Logging**: Emoji-enhanced logs for easy visual scanning
16. **Debug Mode**: Safe testing environment that simulates API calls


## Dependencies

- `github.com/chromedp/chromedp` - Browser automation
- `github.com/joho/godotenv` - Environment variable loading

## License

MIT License
