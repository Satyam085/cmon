package sfms

import (
	"context"
	"fmt"
	"html"
	"log"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"cmon/internal/storage"
)

// WSRefreshBroadcaster allows broadcasting real-time refresh signals to connected clients.
type WSRefreshBroadcaster interface {
	BroadcastRefresh()
}

// DashboardFeederItem represents an individual feeder in the dashboard payload.
type DashboardFeederItem struct {
	FID              int    `json:"fid"`
	Name             string `json:"name"`
	CleanName        string `json:"clean_name"`
	Category         int    `json:"category"`
	CategoryName     string `json:"category_name"`
	Is24x7           bool   `json:"is_24x7"`
	ScheduleStart    string `json:"schedule_start"`
	ScheduleEnd      string `json:"schedule_end"`
	SubstationID     int    `json:"substation_id"`
	SubstationName   string `json:"substation_name"`
	BMUSerialNo      string `json:"bmu_serial_no"`
	FdrCode          string `json:"fdr_code"`
	Device           string `json:"device"`
	Seq              int    `json:"seq"`
	IsActive         bool   `json:"is_active"`
	BMUIsActive      bool   `json:"bmu_is_active"`
	CBON             int    `json:"cbon"`
	CBOFF            int    `json:"cboff"`
	HasTelemetry     bool   `json:"has_telemetry"`
	BreakerStatus    string `json:"breaker_status"` // "CLOSED", "OPEN", "UNKNOWN"
	IsOnline         bool   `json:"is_online"`
	IsDormant        bool   `json:"is_dormant"`
	InterruptedSince string `json:"interrupted_since,omitempty"`
	Downtime         string `json:"downtime,omitempty"`
}

// DashboardSubstationGroup represents a group of feeders belonging to a substation.
type DashboardSubstationGroup struct {
	SSID        int                   `json:"ssid"`
	Name        string                `json:"name"`
	CleanName   string                `json:"clean_name"`
	Total       int                   `json:"total"`
	ActiveCount int                   `json:"active_count"`
	DownCount   int                   `json:"down_count"`
	Feeders     []DashboardFeederItem `json:"feeders"`
}

// DashboardSummary represents the top-level stats in the dashboard.
type DashboardSummary struct {
	TotalSubstations int `json:"total_substations"`
	TotalFeeders     int `json:"total_feeders"`
	ActiveOnline     int `json:"active_online"`
	InterruptedDown  int `json:"interrupted_down"`
	Dormant          int `json:"dormant"`
	TelemetryActive  int `json:"telemetry_active"`
}

// DashboardPayload is the JSON structure returned by GET /sfms/data.
type DashboardPayload struct {
	GeneratedAt    string                     `json:"generated_at"`
	TokenActive    bool                       `json:"token_active"`
	TokenError     string                     `json:"token_error,omitempty"`
	TokenUpdatedAt string                     `json:"token_updated_at,omitempty"`
	Summary        DashboardSummary           `json:"summary"`
	Groups         []DashboardSubstationGroup `json:"groups"`
	Events         []storage.SFMSEvent        `json:"events"`
}

// Monitor coordinates SFMS REST API polling, real-time MQTT telemetry diffing, and alerting.
type Monitor struct {
	config            *Config
	sfmsClient        *Client
	telemetry         *TelemetryClient
	notifier          *Notifier
	stor              *storage.Storage
	wsHub             WSRefreshBroadcaster
	mu                sync.RWMutex
	cachedSubstations []Substation
	states            map[int]*FeederState
	isFirstRun        bool
	authFailed        bool
	lastAuthError     string
	tokenUpdatedAt    time.Time

	debounceMu    sync.Mutex
	debounceTimer *time.Timer
}

// NewMonitor initializes a new SFMS Monitor instance.
func NewMonitor(
	cfg *Config,
	client *Client,
	telemetry *TelemetryClient,
	notifier *Notifier,
	stor *storage.Storage,
	wsHub WSRefreshBroadcaster,
) *Monitor {
	var tokenUpdatedAt time.Time
	if stor != nil {
		if t, err := stor.GetSFMSTokenUpdatedAt(); err == nil && !t.IsZero() {
			tokenUpdatedAt = t
		}
	}
	if tokenUpdatedAt.IsZero() {
		if fi, err := os.Stat("token.txt"); err == nil {
			tokenUpdatedAt = fi.ModTime()
		} else if fi, err := os.Stat("smfs/token.txt"); err == nil {
			tokenUpdatedAt = fi.ModTime()
		}
	}

	states := make(map[int]*FeederState)
	if stor != nil {
		if savedFeeders, err := stor.GetSFMSFeeders(); err == nil {
			for _, f := range savedFeeders {
				if f.FID > 0 {
					var intSince *time.Time
					if f.InterruptedSince != nil && !f.InterruptedSince.IsZero() {
						t := f.InterruptedSince.In(IST)
						intSince = &t
					}
					states[f.FID] = &FeederState{
						FID:              f.FID,
						Name:             f.Name,
						Category:         f.Category,
						CategoryName:     f.CategoryName,
						Is24x7:           f.Is24x7,
						ScheduleStart:    f.ScheduleStart,
						ScheduleEnd:      f.ScheduleEnd,
						SubstationID:     f.SubstationID,
						SubstationName:   f.SubstationName,
						BMUSerialNo:      f.BMUSerialNo,
						FdrCode:          f.FdrCode,
						Device:           f.Device,
						Seq:              f.Seq,
						IsActive:         f.IsActive,
						BMUIsActive:      f.BMUIsActive,
						CBON:             f.CBON,
						CBOFF:            f.CBOFF,
						HasTelemetry:     f.HasTelemetry,
						BreakerStatus:    f.BreakerStatus,
						IsOnline:         f.IsOnline,
						InterruptedSince: intSince,
					}
				}
			}
		}
	}

	return &Monitor{
		config:         cfg,
		sfmsClient:     client,
		telemetry:      telemetry,
		notifier:       notifier,
		stor:           stor,
		wsHub:          wsHub,
		states:         states,
		isFirstRun:     true,
		tokenUpdatedAt: tokenUpdatedAt,
	}
}

// broadcastRefresh safely sends a refresh message to the WebSocket hub without panicking.
func (m *Monitor) broadcastRefresh() {
	if m == nil || m.wsHub == nil {
		return
	}
	defer func() {
		if r := recover(); r != nil {
			log.Printf("⚠️  [SFMS] Recovered from wsHub broadcast panic: %v", r)
		}
	}()
	m.wsHub.BroadcastRefresh()
}


// IsFeederInActiveWindow checks if a feeder is currently within its active monitoring schedule.
func (m *Monitor) IsFeederInActiveWindow(fName string, cat int, t time.Time) (bool, bool, string, string, string) {
	clean := strings.ToUpper(CleanFeederName(fName))
	sched, exists := m.config.FeederSchedules[clean]

	var fType, start, end string
	if exists {
		fType = sched.Type
		start = sched.Start
		end = sched.End
	} else {
		fType = FeederCategoryName(cat)
		if cat == 2 {
			start = m.config.AGWindowStart
			if start == "" {
				start = "06:00"
			}
			end = m.config.AGWindowEnd
			if end == "" {
				end = "16:00"
			}
		} else {
			start = "00:00"
			end = "24:00"
		}
	}

	is24x7 := (start == "00:00" && (end == "24:00" || end == "23:59" || end == "" || end == "00:00"))
	if is24x7 {
		return true, true, fType, start, end
	}

	t = t.In(IST)

	startHour, startMin := 0, 0
	endHour, endMin := 24, 0
	if parts := strings.Split(start, ":"); len(parts) >= 2 {
		fmt.Sscanf(parts[0], "%d", &startHour)
		fmt.Sscanf(parts[1], "%d", &startMin)
	}
	if parts := strings.Split(end, ":"); len(parts) >= 2 {
		fmt.Sscanf(parts[0], "%d", &endHour)
		fmt.Sscanf(parts[1], "%d", &endMin)
	}

	startMinutes := startHour*60 + startMin
	endMinutes := endHour*60 + endMin
	currentMinutes := t.Hour()*60 + t.Minute()

	isActive := false
	if startMinutes <= endMinutes {
		isActive = currentMinutes >= startMinutes && currentMinutes < endMinutes
	} else {
		isActive = currentMinutes >= startMinutes || currentMinutes < endMinutes
	}

	return isActive, false, fType, start, end
}

func isAuthError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "401") ||
		strings.Contains(msg, "403") ||
		strings.Contains(msg, "unauthorized") ||
		strings.Contains(msg, "token invalidated") ||
		strings.Contains(msg, "bearer token is empty")
}

// SyncMasterData synchronizes the substation and feeder inventory from GETCO SFMS REST API.
func (m *Monitor) SyncMasterData(ctx context.Context) error {
	substations, err := m.sfmsClient.FetchSubstations(ctx)
	if err != nil {
		if isAuthError(err) {
			m.mu.Lock()
			m.lastAuthError = err.Error()
			if !m.authFailed {
				m.authFailed = true
				m.notifier.SendAuthError(ctx, err.Error())
			}
			m.mu.Unlock()
		} else {
			log.Printf("[%s] ⚠️  [SFMS] Master data fetch warning: %v", FormatNowIST(), err)
		}
		return fmt.Errorf("failed to fetch substations: %w", err)
	}

	m.mu.Lock()
	if m.authFailed {
		m.authFailed = false
		m.lastAuthError = ""
		m.notifier.SendAuthRestored(ctx)
	}
	m.cachedSubstations = substations
	m.mu.Unlock()

	// Update real-time MQTT subscriptions for all substation devices
	if m.telemetry != nil {
		_ = m.telemetry.UpdateSubscriptions(ctx, substations)
	}

	return nil
}

// OnTelemetryUpdate is triggered debounced when MQTT telemetry messages change breaker states.
func (m *Monitor) OnTelemetryUpdate(ctx context.Context) {
	m.debounceMu.Lock()
	defer m.debounceMu.Unlock()

	if m.debounceTimer != nil {
		m.debounceTimer.Stop()
	}

	m.debounceTimer = time.AfterFunc(150*time.Millisecond, func() {
		_ = m.EvaluateFeederStates(ctx, false)
	})
}

// EvaluateFeederStates evaluates live feeder states using cached inventory and real-time MQTT telemetry.
func (m *Monitor) EvaluateFeederStates(ctx context.Context, printSummary bool) error {
	m.mu.Lock()
	substations := m.cachedSubstations
	if len(substations) == 0 {
		m.mu.Unlock()
		if err := m.SyncMasterData(ctx); err != nil {
			return err
		}
		m.mu.Lock()
		substations = m.cachedSubstations
	}
	defer m.mu.Unlock()

	now := NowIST()
	timestampStr := FormatTimeIST(now)

	total24x7Feeders := 0
	active24x7Feeders := 0
	interrupted24x7Feeders := 0

	totalScheduledFeeders := 0
	activeScheduledFeeders := 0
	interruptedScheduledFeeders := 0
	dormantScheduledFeeders := 0

	var newInterruptionAlerts []string
	var newRecoveryAlerts []string
	stateHasChanged := false

	var feederRecordsToSave []storage.SFMSFeederRecord

	for _, ss := range substations {
		if len(m.config.FilterSubstations) > 0 && !m.isSubstationFiltered(ss.Name) {
			continue
		}

		for _, f := range ss.FeederInfo {
			if len(m.config.FilterFeeders) > 0 && !m.isFeederFiltered(f.Name) {
				continue
			}

			isActiveWindow, is24x7, fType, start, end := m.IsFeederInActiveWindow(f.Name, f.Cat, now)

			if is24x7 {
				total24x7Feeders++
			} else {
				totalScheduledFeeders++
			}

			prev, exists := m.states[f.FID]

			// If feeder is outside its schedule window, keep it dormant
			if !isActiveWindow {
				dormantScheduledFeeders++
				if exists && prev.InterruptedSince != nil {
					prev.InterruptedSince = nil
				}
				continue
			}

			// Real-time circuit breaker status from MQTT WebSocket telemetry
			var isOnline bool
			var cbon, cboff int
			var breakerStatus string
			var hasTelemetry bool

			if m.telemetry != nil && f.Device != "" && f.Seq > 0 {
				cbon, cboff, breakerStatus, hasTelemetry, _ = m.telemetry.GetBreakerState(f.Device, f.Seq)
			}

			if hasTelemetry {
				isOnline = (breakerStatus == "CLOSED" || (cbon == 1 && cboff == 0))
			} else {
				isOnline = f.ISACT && f.BMUISACT
			}

			if is24x7 {
				if isOnline {
					active24x7Feeders++
				} else {
					interrupted24x7Feeders++
				}
			} else {
				if isOnline {
					activeScheduledFeeders++
				} else {
					interruptedScheduledFeeders++
				}
			}

			isInitial := !exists
			if isInitial {
				var interruptedSince *time.Time
				if !isOnline {
					t := now
					interruptedSince = &t
				}
				m.states[f.FID] = &FeederState{
					FID:              f.FID,
					Name:             f.Name,
					Category:         f.Cat,
					CategoryName:     fType,
					Is24x7:           is24x7,
					ScheduleStart:    start,
					ScheduleEnd:      end,
					SubstationID:     ss.SSID,
					SubstationName:   ss.Name,
					BMUSerialNo:      f.BMUSerialNo,
					FdrCode:          f.FdrCode,
					Device:           f.Device,
					Seq:              f.Seq,
					IsActive:         f.ISACT,
					BMUIsActive:      f.BMUISACT,
					CBON:             cbon,
					CBOFF:            cboff,
					HasTelemetry:     hasTelemetry,
					BreakerStatus:    breakerStatus,
					IsOnline:         isOnline,
					InterruptedSince: interruptedSince,
				}

				if !isOnline && !m.isFirstRun {
					m.notifier.SendInterruption(ctx, f.Name, ss.Name, f.FID, f.FdrCode, f.BMUSerialNo, fType)
				}
			} else {
				prev.CategoryName = fType
				prev.Is24x7 = is24x7
				prev.ScheduleStart = start
				prev.ScheduleEnd = end
				prev.Device = f.Device
				prev.Seq = f.Seq
				prev.CBON = cbon
				prev.CBOFF = cboff
				prev.HasTelemetry = hasTelemetry
				prev.BreakerStatus = breakerStatus

				// State evaluation & transition detection
				if prev.IsOnline && !isOnline {
					// TRANSITION: Online -> Interrupted
					t := now
					prev.InterruptedSince = &t
					prev.IsOnline = false
					prev.IsActive = f.ISACT
					prev.BMUIsActive = f.BMUISACT

					stateHasChanged = true
					newInterruptionAlerts = append(newInterruptionAlerts, f.Name)
					m.notifier.SendInterruption(ctx, f.Name, ss.Name, f.FID, f.FdrCode, f.BMUSerialNo, fType)
				} else if !prev.IsOnline && isOnline {
					// TRANSITION: Interrupted -> Restored
					var downtimeStr = "unknown"
					if prev.InterruptedSince != nil {
						downtimeDuration := now.Sub(*prev.InterruptedSince)
						downtimeStr = formatDuration(downtimeDuration)
					}

					prev.InterruptedSince = nil
					prev.IsOnline = true
					prev.IsActive = f.ISACT
					prev.BMUIsActive = f.BMUISACT

					stateHasChanged = true
					newRecoveryAlerts = append(newRecoveryAlerts, f.Name)
					m.notifier.SendRecovery(ctx, f.Name, ss.Name, f.FID, downtimeStr, fType)
				} else {
					prev.IsActive = f.ISACT
					prev.BMUIsActive = f.BMUISACT
					if !isOnline && prev.InterruptedSince == nil {
						t := now
						prev.InterruptedSince = &t
					}
				}
			}

			// Build record for SQLite
			st := m.states[f.FID]
			feederRecordsToSave = append(feederRecordsToSave, storage.SFMSFeederRecord{
				FID:              st.FID,
				Name:             st.Name,
				Category:         st.Category,
				CategoryName:     st.CategoryName,
				Is24x7:           st.Is24x7,
				ScheduleStart:    st.ScheduleStart,
				ScheduleEnd:      st.ScheduleEnd,
				SubstationID:     st.SubstationID,
				SubstationName:   st.SubstationName,
				BMUSerialNo:      st.BMUSerialNo,
				FdrCode:          st.FdrCode,
				Device:           st.Device,
				Seq:              st.Seq,
				IsActive:         st.IsActive,
				BMUIsActive:      st.BMUIsActive,
				CBON:             st.CBON,
				CBOFF:            st.CBOFF,
				HasTelemetry:     st.HasTelemetry,
				BreakerStatus:    st.BreakerStatus,
				IsOnline:         st.IsOnline,
				InterruptedSince: st.InterruptedSince,
				UpdatedAt:        now,
			})
		}
	}

	// Persist to SQLite
	if m.stor != nil && len(feederRecordsToSave) > 0 {
		_ = m.stor.SaveSFMSFeeders(feederRecordsToSave)
	}

	// Broadcast refresh to WebSocket clients if state changed or first run
	if (stateHasChanged || m.isFirstRun) {
		m.broadcastRefresh()
	}

	if printSummary || len(newInterruptionAlerts) > 0 || len(newRecoveryAlerts) > 0 {
		totalMonitoredNow := active24x7Feeders + interrupted24x7Feeders + activeScheduledFeeders + interruptedScheduledFeeders
		totalActiveNow := active24x7Feeders + activeScheduledFeeders
		totalInterruptedNow := interrupted24x7Feeders + interruptedScheduledFeeders

		log.Printf("[%s] ⚡ [SFMS] Substations: %d | Monitored: %d | Online: %d | Interrupted: %d (Dormant: %d)",
			timestampStr, len(substations), totalMonitoredNow, totalActiveNow, totalInterruptedNow, dormantScheduledFeeders)
	}

	if m.isFirstRun {
		m.isFirstRun = false
	}

	return nil
}

// UpdateTokenAndVerify updates the Bearer token, verifies it against GETCO API, and saves it.
func (m *Monitor) UpdateTokenAndVerify(ctx context.Context, rawToken string) (int, error) {
	norm := NormalizeBearerToken(rawToken)
	if norm == "" {
		return 0, fmt.Errorf("token cannot be empty")
	}

	count, err := m.sfmsClient.VerifyToken(ctx, norm)
	if err != nil {
		return 0, fmt.Errorf("token verification failed: %w", err)
	}

	m.sfmsClient.SetToken(norm)
	m.config.BearerToken = norm

	_ = os.WriteFile("token.txt", []byte(norm), 0644)
	if m.stor != nil {
		_ = m.stor.SetSFMSToken(norm)
	}

	now := time.Now()
	m.mu.Lock()
	m.tokenUpdatedAt = now
	if m.authFailed {
		m.authFailed = false
		m.lastAuthError = ""
		m.notifier.SendAuthRestored(ctx)
	}
	m.mu.Unlock()

	go func() {
		_ = m.SyncMasterData(context.Background())
		_ = m.EvaluateFeederStates(context.Background(), true)
	}()

	return count, nil
}

// BuildStatusReportHTML builds the full feeder status report formatted in HTML for Telegram.
func (m *Monitor) BuildStatusReportHTML() string {
	m.mu.RLock()
	substations := m.cachedSubstations
	states := m.states
	authFailed := m.authFailed
	m.mu.RUnlock()

	now := NowIST()
	ts := FormatTimeIST(now)

	if len(substations) == 0 {
		if authFailed {
			return fmt.Sprintf("⚠️ <b>SFMS Feeder Status</b>\n\n❌ <i>Authentication token expired. Please update token at /sfms.</i>\nTime: %s", ts)
		}
		return fmt.Sprintf("ℹ️ <b>SFMS Feeder Status</b>\n\nNo substation data loaded yet. Please wait for initial sync.\nTime: %s", ts)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("⚡ <b>GETCO SFMS Feeder Status Report</b>\n📅 <i>%s</i>\n\n", ts))

	totalFeeders := 0
	totalOnline := 0
	totalInterrupted := 0
	totalDormant := 0

	var ssReports []string
	validSSCount := 0

	for _, ss := range substations {
		if len(m.config.FilterSubstations) > 0 && !m.isSubstationFiltered(ss.Name) {
			continue
		}

		cleanSS := CleanSubstationName(ss.Name)
		var lines []string

		for _, f := range ss.FeederInfo {
			if len(m.config.FilterFeeders) > 0 && !m.isFeederFiltered(f.Name) {
				continue
			}

			totalFeeders++
			cleanF := CleanFeederName(f.Name)
			isActiveWindow, is24x7, fType, _, _ := m.IsFeederInActiveWindow(f.Name, f.Cat, now)

			st, hasState := states[f.FID]
			if !isActiveWindow {
				totalDormant++
				lines = append(lines, fmt.Sprintf("  🟡 %s [%s] (Dormant)", html.EscapeString(cleanF), html.EscapeString(fType)))
				continue
			}

			isOnline := false
			if hasState {
				isOnline = st.IsOnline
			} else {
				isOnline = f.ISACT && f.BMUISACT
			}

			if isOnline {
				totalOnline++
				lines = append(lines, fmt.Sprintf("  🟢 %s [%s]", html.EscapeString(cleanF), html.EscapeString(fType)))
			} else {
				totalInterrupted++
				downtime := ""
				if hasState && st.InterruptedSince != nil {
					downtime = fmt.Sprintf(" (Down: %s)", formatDuration(now.Sub(*st.InterruptedSince)))
				}
				lines = append(lines, fmt.Sprintf("  🔴 <b>%s</b> [%s]%s", html.EscapeString(cleanF), html.EscapeString(fType), html.EscapeString(downtime)))
			}
			_ = is24x7
		}

		if len(lines) > 0 {
			validSSCount++
			ssBlock := fmt.Sprintf("<b>Substation: %s</b>\n%s", html.EscapeString(cleanSS), strings.Join(lines, "\n"))
			ssReports = append(ssReports, ssBlock)
		}
	}

	sb.WriteString(fmt.Sprintf(
		"📊 <b>Summary:</b> %d Substations | %d Feeders\n"+
			"🟢 <b>Online:</b> %d | 🔴 <b>Interrupted:</b> %d | 🟡 <b>Dormant:</b> %d\n\n",
		validSSCount, totalFeeders, totalOnline, totalInterrupted, totalDormant,
	))

	sb.WriteString(strings.Join(ssReports, "\n\n"))
	return sb.String()
}

// BuildStatusReportText builds the full feeder status report in plain text for WhatsApp.
func (m *Monitor) BuildStatusReportText() string {
	m.mu.RLock()
	substations := m.cachedSubstations
	states := m.states
	authFailed := m.authFailed
	m.mu.RUnlock()

	now := NowIST()
	ts := FormatTimeIST(now)

	if len(substations) == 0 {
		if authFailed {
			return fmt.Sprintf("⚠️ SFMS Feeder Status\n\n❌ Authentication token expired. Please update token at /sfms.\nTime: %s", ts)
		}
		return fmt.Sprintf("ℹ️ SFMS Feeder Status\n\nNo substation data loaded yet. Please wait for initial sync.\nTime: %s", ts)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("⚡ GETCO SFMS Feeder Status Report\nTime: %s\n\n", ts))

	totalFeeders := 0
	totalOnline := 0
	totalInterrupted := 0
	totalDormant := 0

	var ssReports []string
	validSSCount := 0

	for _, ss := range substations {
		if len(m.config.FilterSubstations) > 0 && !m.isSubstationFiltered(ss.Name) {
			continue
		}

		cleanSS := CleanSubstationName(ss.Name)
		var lines []string

		for _, f := range ss.FeederInfo {
			if len(m.config.FilterFeeders) > 0 && !m.isFeederFiltered(f.Name) {
				continue
			}

			totalFeeders++
			cleanF := CleanFeederName(f.Name)
			isActiveWindow, _, fType, _, _ := m.IsFeederInActiveWindow(f.Name, f.Cat, now)

			st, hasState := states[f.FID]
			if !isActiveWindow {
				totalDormant++
				lines = append(lines, fmt.Sprintf("  🟡 %s [%s] (Dormant)", cleanF, fType))
				continue
			}

			isOnline := false
			if hasState {
				isOnline = st.IsOnline
			} else {
				isOnline = f.ISACT && f.BMUISACT
			}

			if isOnline {
				totalOnline++
				lines = append(lines, fmt.Sprintf("  🟢 %s [%s]", cleanF, fType))
			} else {
				totalInterrupted++
				downtime := ""
				if hasState && st.InterruptedSince != nil {
					downtime = fmt.Sprintf(" (Down: %s)", formatDuration(now.Sub(*st.InterruptedSince)))
				}
				lines = append(lines, fmt.Sprintf("  🔴 %s [%s]%s", cleanF, fType, downtime))
			}
		}

		if len(lines) > 0 {
			validSSCount++
			ssBlock := fmt.Sprintf("Substation: %s\n%s", cleanSS, strings.Join(lines, "\n"))
			ssReports = append(ssReports, ssBlock)
		}
	}

	sb.WriteString(fmt.Sprintf(
		"Summary: %d Substations | %d Feeders\n"+
			"🟢 Online: %d | 🔴 Interrupted: %d | 🟡 Dormant: %d\n\n",
		validSSCount, totalFeeders, totalOnline, totalInterrupted, totalDormant,
	))

	sb.WriteString(strings.Join(ssReports, "\n\n"))
	return sb.String()
}

// GetDashboardPayload constructs the full JSON payload for GET /sfms/data.
func (m *Monitor) GetDashboardPayload() DashboardPayload {
	m.mu.RLock()
	defer m.mu.RUnlock()

	now := NowIST()
	summary := DashboardSummary{}
	var groups []DashboardSubstationGroup

	for _, ss := range m.cachedSubstations {
		if len(m.config.FilterSubstations) > 0 && !m.isSubstationFiltered(ss.Name) {
			continue
		}

		cleanSS := CleanSubstationName(ss.Name)
		grp := DashboardSubstationGroup{
			SSID:      ss.SSID,
			Name:      ss.Name,
			CleanName: cleanSS,
			Feeders:   make([]DashboardFeederItem, 0, len(ss.FeederInfo)),
		}

		for _, f := range ss.FeederInfo {
			if len(m.config.FilterFeeders) > 0 && !m.isFeederFiltered(f.Name) {
				continue
			}

			summary.TotalFeeders++
			cleanF := CleanFeederName(f.Name)
			isActiveWindow, is24x7, fType, start, end := m.IsFeederInActiveWindow(f.Name, f.Cat, now)

			st, hasState := m.states[f.FID]
			isOnline := false
			hasTelemetry := false
			breakerStatus := "UNKNOWN"
			cbon := 0
			cboff := 0
			intSinceStr := ""
			downtimeStr := ""

			if hasState {
				isOnline = st.IsOnline
				hasTelemetry = st.HasTelemetry
				breakerStatus = st.BreakerStatus
				cbon = st.CBON
				cboff = st.CBOFF
				if st.InterruptedSince != nil {
					intSinceStr = st.InterruptedSince.In(IST).Format("02-01-06 15:04:05")
					downtimeStr = formatDuration(now.Sub(*st.InterruptedSince))
				}
			} else {
				isOnline = f.ISACT && f.BMUISACT
			}

			if hasTelemetry {
				summary.TelemetryActive++
			}

			isDormant := !isActiveWindow
			if isDormant {
				summary.Dormant++
			} else if isOnline {
				summary.ActiveOnline++
				grp.ActiveCount++
			} else {
				summary.InterruptedDown++
				grp.DownCount++
			}

			grp.Feeders = append(grp.Feeders, DashboardFeederItem{
				FID:              f.FID,
				Name:             f.Name,
				CleanName:        cleanF,
				Category:         f.Cat,
				CategoryName:     fType,
				Is24x7:           is24x7,
				ScheduleStart:    start,
				ScheduleEnd:      end,
				SubstationID:     ss.SSID,
				SubstationName:   cleanSS,
				BMUSerialNo:      f.BMUSerialNo,
				FdrCode:          f.FdrCode,
				Device:           f.Device,
				Seq:              f.Seq,
				IsActive:         f.ISACT,
				BMUIsActive:      f.BMUISACT,
				CBON:             cbon,
				CBOFF:            cboff,
				HasTelemetry:     hasTelemetry,
				BreakerStatus:    breakerStatus,
				IsOnline:         isOnline,
				IsDormant:        isDormant,
				InterruptedSince: intSinceStr,
				Downtime:         downtimeStr,
			})
		}

		if len(grp.Feeders) > 0 {
			grp.Total = len(grp.Feeders)
			groups = append(groups, grp)
		}
	}

	summary.TotalSubstations = len(groups)

	sort.Slice(groups, func(i, j int) bool {
		return groups[i].CleanName < groups[j].CleanName
	})

	var recentEvents []storage.SFMSEvent
	if m.stor != nil {
		recentEvents, _ = m.stor.GetSFMSEvents(25)
	}

	tokenActive := !m.authFailed && m.config.BearerToken != ""
	tokenUpdatedStr := ""
	var tokenTime time.Time
	if m.stor != nil {
		if t, err := m.stor.GetSFMSTokenUpdatedAt(); err == nil && !t.IsZero() {
			tokenTime = t
		}
	}
	if tokenTime.IsZero() && !m.tokenUpdatedAt.IsZero() {
		tokenTime = m.tokenUpdatedAt
	}
	if !tokenTime.IsZero() {
		tokenUpdatedStr = tokenTime.In(IST).Format("02 Jan 2006, 03:04 PM")
	}

	return DashboardPayload{
		GeneratedAt:    FormatTimeIST(now),
		TokenActive:    tokenActive,
		TokenError:     m.lastAuthError,
		TokenUpdatedAt: tokenUpdatedStr,
		Summary:        summary,
		Groups:         groups,
		Events:         recentEvents,
	}
}

func (m *Monitor) isSubstationFiltered(name string) bool {
	clean := strings.ToLower(CleanSubstationName(name))
	for _, f := range m.config.FilterSubstations {
		if strings.ToLower(CleanSubstationName(f)) == clean {
			return true
		}
	}
	return false
}

func (m *Monitor) isFeederFiltered(name string) bool {
	clean := strings.ToLower(CleanFeederName(name))
	for _, f := range m.config.FilterFeeders {
		if strings.ToLower(CleanFeederName(f)) == clean {
			return true
		}
	}
	return false
}

func formatDuration(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	d = d.Round(time.Second)
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	if h > 0 {
		return fmt.Sprintf("%dh %dm %ds", h, m, s)
	}
	if m > 0 {
		return fmt.Sprintf("%dm %ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}
