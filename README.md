# CMON - Complaint Monitoring System

Automated complaint monitoring and notification system for DGVCL (Dakshin Gujarat Vij Company Limited). Scrapes complaint data from the DGVCL portal and sends real-time notifications via Telegram.

## Features

- Automated periodic fetching of complaints
- Pagination support for large datasets
- Real-time Telegram notifications with formatted messages
- Resolved complaint tracking (edits original message as resolved)
- Automatic session management with retry logic
- Health check endpoint for monitoring

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

Create a `.env` file with the following variables:

```env
# Telegram Configuration
TELEGRAM_BOT_TOKEN=your_bot_token
TELEGRAM_CHAT_ID=your_chat_id

# DGVCL Authentication
DGVCL_USERNAME=your_username
DGVCL_PASSWORD=your_password

# Application Settings
FETCH_INTERVAL=15m
MAX_LOGIN_RETRIES=3
MAX_FETCH_RETRIES=2
```

### Environment Variables Reference

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `DGVCL_USERNAME` | Yes | - | Portal username |
| `DGVCL_PASSWORD` | Yes | - | Portal password |
| `TELEGRAM_BOT_TOKEN` | No | - | Telegram bot token |
| `TELEGRAM_CHAT_ID` | No | - | Telegram chat ID |
| `FETCH_INTERVAL` | No | 15m | Fetch frequency |
| `MAX_LOGIN_RETRIES` | No | 3 | Login retry attempts |
| `LOGIN_RETRY_DELAY` | No | 5s | Delay between login retries |
| `HEALTH_CHECK_PORT` | No | 8080 | Health check server port |

## Project Structure

```
cmon/
├── main.go              # Entry point, fetch loop, error handling
├── complaint.go         # Complaint fetching & pagination logic
├── login.go             # Authentication handling
├── browser.go           # ChromeDP browser context management
├── storage.go           # CSV-based complaint storage with message IDs
├── telegram.go          # Telegram API integration & message editing
├── config.go            # Configuration loading from env vars
├── http_client.go       # Shared HTTP client for API calls
├── errors.go            # Custom error types
├── complaints.csv       # Persistent storage (auto-generated)
└── .env                 # Configuration (not committed)
```

## Code Execution Flow

```
┌─────────────────────────────────────────────────────────────────┐
│                        main()                                    │
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
│     - Retry up to MAX_LOGIN_RETRIES on failure                 │
└─────────────────────────────────────────────────────────────────┘
                                │
                                ▼
┌─────────────────────────────────────────────────────────────────┐
│  4. Initial Fetch (FetchComplaints)                             │
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
│  5. markResolvedComplaints()                                    │
│     ├─ Get all previously stored complaints                     │
│     ├─ Compare with current website complaints                 │
│     ├─ For resolved (not in current):                          │
│     │  ├─ Edit Telegram message (add "RESOLVED" header)     │
│     │  └─ Remove from complaints.csv                        │
└─────────────────────────────────────────────────────────────────┘
                                │
                                ▼
┌─────────────────────────────────────────────────────────────────┐
│  6. Refresh Loop (every FETCH_INTERVAL)                         │
│     ├─ fetchWithRetry()                                         │
│     │  ├─ FetchComplaints()                                    │
│     │  ├─ If session expired → re-login → retry                │
│     │  └─ If re-login fails → restart browser → retry          │
│     └─ markResolvedComplaints()                                 │
└─────────────────────────────────────────────────────────────────┘
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

```bash
# Build and run
go build -o cmon .
./cmon

# Or with go run
go run .
```

## Common Issues & Solutions

| Issue | Solution |
|-------|----------|
| Login fails repeatedly | Check credentials in .env file, ensure portal is accessible |
| ChromeDP crashes | Ensure Chrome/Chromium is installed on the system |
| Telegram not sending | Verify bot token and chat ID are correct |
| Missing complaints | Check pagination logic, increase timeout values |
| Session expired errors | Normal behavior; system auto-relogs |
| Browser memory growth | Restart interval will help; consider containerized deployment |

## Architecture Highlights

1. **Browser Automation**: Uses ChromeDP for headless Chrome automation
2. **Pagination Detection**: Checks for "Next" button in DOM to determine last page
3. **Session Management**: Automatic re-login and browser restart on session expiry
4. **Error Recovery**: Three-tier retry logic (retry → re-login → restart browser)
5. **Message Tracking**: Stores Telegram message IDs to enable future edits
6. **Graceful Shutdown**: Handles SIGTERM for clean shutdown

## Dependencies

- `github.com/chromedp/chromedp` - Browser automation
- `github.com/joho/godotenv` - Environment variable loading

## License

MIT License
