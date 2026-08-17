package sfms

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

// TelemetryClient manages real-time MQTT over WebSocket telemetry from GETCO SCADA broker.
type TelemetryClient struct {
	cfg        *Config
	sfmsClient *Client
	mqttClient mqtt.Client
	mu         sync.RWMutex

	brokerURL string
	username  string
	password  string
	vHost     string

	// grpID -> map[uuid]TagConfig
	groupConfigs map[string]map[string]TagConfig
	// deviceID -> grpID
	deviceToGroup map[string]string
	// grpID -> deviceID
	groupToDevice map[string]string
	// "deviceID:seq" -> LiveBreakerState
	breakers map[string]LiveBreakerState
	// Set of currently subscribed group IDs
	subscribedGroups map[string]bool

	isConnected bool
	onUpdate    func()
}

// NewTelemetryClient creates and initializes a TelemetryClient.
func NewTelemetryClient(cfg *Config, sfmsClient *Client) *TelemetryClient {
	tc := &TelemetryClient{
		cfg:              cfg,
		sfmsClient:       sfmsClient,
		brokerURL:        "wss://ebrokersfms.hkapl.in:15673/ws",
		username:         "hkrpadmin",
		password:         "hkrpadmin@2021",
		vHost:            "map",
		groupConfigs:     make(map[string]map[string]TagConfig),
		deviceToGroup:    make(map[string]string),
		groupToDevice:    make(map[string]string),
		breakers:         make(map[string]LiveBreakerState),
		subscribedGroups: make(map[string]bool),
	}
	return tc
}

// SetOnUpdate registers a callback triggered when new telemetry arrives.
func (tc *TelemetryClient) SetOnUpdate(fn func()) {
	tc.mu.Lock()
	defer tc.mu.Unlock()
	tc.onUpdate = fn
}

// IsConnected returns whether the MQTT broker is currently connected.
func (tc *TelemetryClient) IsConnected() bool {
	tc.mu.RLock()
	defer tc.mu.RUnlock()
	return tc.isConnected
}

// Start connects to the MQTT broker over WebSocket and starts processing messages.
func (tc *TelemetryClient) Start(ctx context.Context) error {
	opts := mqtt.NewClientOptions()
	opts.AddBroker(tc.brokerURL)
	opts.SetClientID(fmt.Sprintf("sfms-cmon-%d", time.Now().UnixNano()%1000000))
	opts.SetUsername(fmt.Sprintf("%s:%s", tc.vHost, tc.username))
	opts.SetPassword(tc.password)
	opts.SetTLSConfig(&tls.Config{InsecureSkipVerify: true})
	opts.SetHTTPHeaders(http.Header{
		"Origin": []string{"https://getco-sfms.in"},
	})
	opts.SetCleanSession(true)
	opts.SetKeepAlive(20 * time.Second)
	opts.SetPingTimeout(10 * time.Second)
	opts.SetAutoReconnect(true)
	opts.SetMaxReconnectInterval(5 * time.Second)

	opts.OnConnect = func(c mqtt.Client) {
		tc.mu.Lock()
		tc.isConnected = true
		tc.mu.Unlock()
		log.Printf("[%s] 🌐 [WEBSOCKET] Connected to GETCO SFMS real-time MQTT broker (%s)",
			FormatNowIST(), tc.brokerURL)

		// Resubscribe to existing groups on reconnect
		tc.resubscribeAll()
	}

	opts.OnConnectionLost = func(c mqtt.Client, err error) {
		tc.mu.Lock()
		tc.isConnected = false
		tc.mu.Unlock()
		log.Printf("[%s] ⚠️ [WEBSOCKET] MQTT connection lost: %v. Auto-reconnecting...",
			FormatNowIST(), err)
	}

	tc.mqttClient = mqtt.NewClient(opts)
	token := tc.mqttClient.Connect()
	if token.Wait() && token.Error() != nil {
		return fmt.Errorf("MQTT over WebSocket connection failed: %w", token.Error())
	}

	go func() {
		<-ctx.Done()
		tc.Stop()
	}()

	return nil
}

// Stop disconnects the telemetry client.
func (tc *TelemetryClient) Stop() {
	if tc.mqttClient != nil && tc.mqttClient.IsConnected() {
		tc.mqttClient.Disconnect(250)
	}
}

// UpdateSubscriptions queries device mapping groups from GETCO SFMS and subscribes to telemetry streams.
func (tc *TelemetryClient) UpdateSubscriptions(ctx context.Context, substations []Substation) error {
	var deviceIDs []string
	deviceSet := make(map[string]bool)

	for _, ss := range substations {
		for _, f := range ss.FeederInfo {
			if f.Device != "" && !deviceSet[f.Device] {
				deviceSet[f.Device] = true
				deviceIDs = append(deviceIDs, f.Device)
			}
		}
	}

	if len(deviceIDs) == 0 {
		return nil
	}

	groups, err := tc.sfmsClient.FetchDeviceMappedGroups(ctx, "SS-IOMONITORING", deviceIDs)
	if err != nil {
		return fmt.Errorf("failed to fetch device mapped groups: %w", err)
	}

	tc.mu.Lock()
	for _, g := range groups {
		if g.GrpID != "" && g.Did != "" {
			tc.deviceToGroup[g.Did] = g.GrpID
			tc.groupToDevice[g.GrpID] = g.Did
		}
	}
	tc.mu.Unlock()

	tc.resubscribeAll()
	return nil
}

func (tc *TelemetryClient) resubscribeAll() {
	if tc.mqttClient == nil || !tc.mqttClient.IsConnected() {
		return
	}

	tc.mu.Lock()
	groupsToSub := make([]string, 0, len(tc.groupToDevice))
	for grpID := range tc.groupToDevice {
		groupsToSub = append(groupsToSub, grpID)
	}
	tc.mu.Unlock()

	for _, grpID := range groupsToSub {
		tc.subscribeGroup(grpID)
	}
}

func (tc *TelemetryClient) subscribeGroup(grpID string) {
	configTopic := fmt.Sprintf("%s/%s/config", tc.vHost, grpID)
	valuesTopic := fmt.Sprintf("%s/%s/values", tc.vHost, grpID)

	topics := map[string]byte{
		configTopic: 0,
		valuesTopic: 0,
	}

	tc.mqttClient.SubscribeMultiple(topics, func(c mqtt.Client, m mqtt.Message) {
		tc.handleMessage(m.Topic(), m.Payload())
	})

	tc.mu.Lock()
	tc.subscribedGroups[grpID] = true
	tc.mu.Unlock()
}

func (tc *TelemetryClient) handleMessage(topic string, payload []byte) {
	parts := strings.Split(topic, "/")
	if len(parts) < 3 {
		return
	}
	grpID := parts[1]
	msgType := parts[2]

	payloadStr := string(payload)

	switch msgType {
	case "config":
		configs := parseConfigPayload(payloadStr)
		if len(configs) > 0 {
			tc.mu.Lock()
			tc.groupConfigs[grpID] = configs
			tc.mu.Unlock()
		}
	case "values":
		tc.mu.RLock()
		configs, hasConfig := tc.groupConfigs[grpID]
		did := tc.groupToDevice[grpID]
		tc.mu.RUnlock()

		if !hasConfig || len(configs) == 0 {
			return
		}

		values := parseValuesPayload(payloadStr, configs)
		tc.updateBreakerStates(did, values)
	}
}

func (tc *TelemetryClient) updateBreakerStates(did string, values map[string]TagValue) {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	now := NowIST()
	hasChange := false

	for _, v := range values {
		if strings.HasPrefix(v.NM, "D-") && (strings.Contains(v.NM, "CBON") || strings.Contains(v.NM, "CBOFF")) {
			parts := strings.Split(v.NM, "-")
			if len(parts) >= 3 {
				seq, err := strconv.Atoi(parts[1])
				if err != nil {
					continue
				}

				key := fmt.Sprintf("%s:%d", did, seq)
				curr := tc.breakers[key]
				curr.DeviceID = did
				curr.Seq = seq
				curr.UpdatedAt = now

				if v.Timestamp != "" {
					curr.Timestamp = v.Timestamp
				}

				val, _ := strconv.Atoi(strings.TrimSpace(v.ValStr))
				if strings.Contains(v.NM, "CBON") {
					curr.CBON = val
				} else if strings.Contains(v.NM, "CBOFF") {
					curr.CBOFF = val
				}

				if curr.CBON == 1 && curr.CBOFF == 0 {
					curr.Status = "CLOSED"
				} else if curr.CBON == 0 && curr.CBOFF == 1 {
					curr.Status = "OPEN"
				} else if curr.CBON == 0 && curr.CBOFF == 0 {
					curr.Status = "OPEN"
				} else if curr.CBON == 1 && curr.CBOFF == 1 {
					curr.Status = "ERROR"
				} else {
					curr.Status = "UNKNOWN"
				}

				old, exists := tc.breakers[key]
				if !exists || old.CBON != curr.CBON || old.CBOFF != curr.CBOFF || old.Status != curr.Status {
					hasChange = true
				}

				tc.breakers[key] = curr
			}
		}
	}

	if hasChange && tc.onUpdate != nil {
		go tc.onUpdate()
	}
}

// GetBreakerState returns the current real-time circuit breaker status for a device and sequence.
func (tc *TelemetryClient) GetBreakerState(deviceID string, seq int) (cbon int, cboff int, status string, isLive bool, lastUpdate time.Time) {
	tc.mu.RLock()
	defer tc.mu.RUnlock()

	key := fmt.Sprintf("%s:%d", deviceID, seq)
	b, exists := tc.breakers[key]
	if !exists {
		return 0, 0, "UNKNOWN", false, time.Time{}
	}

	return b.CBON, b.CBOFF, b.Status, true, b.UpdatedAt
}

// IsFeederClosed returns whether the feeder circuit breaker is currently closed (Power ON).
func (tc *TelemetryClient) IsFeederClosed(deviceID string, seq int) (bool, bool) {
	cbon, cboff, status, isLive, _ := tc.GetBreakerState(deviceID, seq)
	if !isLive {
		return false, false
	}
	if status == "CLOSED" || (cbon == 1 && cboff == 0) {
		return true, true
	}
	if status == "OPEN" || (cbon == 0 && cboff == 1) {
		return false, true
	}
	return false, false
}

func parseConfigPayload(payloadStr string) map[string]TagConfig {
	var jsonStr string

	if strings.HasPrefix(payloadStr, "H4s") || strings.HasPrefix(payloadStr, "H4sI") {
		data, err := base64.StdEncoding.DecodeString(payloadStr)
		if err == nil {
			r, err := gzip.NewReader(bytes.NewReader(data))
			if err == nil {
				decomp, _ := io.ReadAll(r)
				jsonStr = string(decomp)
			}
		}
	}

	if jsonStr == "" {
		jsonStr = payloadStr
	}

	var rawList []map[string]string
	if err := json.Unmarshal([]byte(jsonStr), &rawList); err != nil {
		return nil
	}

	configs := make(map[string]TagConfig)
	for _, item := range rawList {
		for uuid, valStr := range item {
			cleanUUID := strings.Trim(uuid, "'\" ")
			parts := strings.Split(valStr, ",")
			cleanParts := make([]string, len(parts))
			for i, p := range parts {
				cleanParts[i] = strings.Trim(p, "'\" ")
			}

			nm := ""
			if len(cleanParts) > 2 {
				nm = cleanParts[2]
			}
			did := ""
			if len(cleanParts) > 0 {
				did = cleanParts[0]
			}
			dnm := ""
			if len(cleanParts) > 1 {
				dnm = cleanParts[1]
			}
			unit := ""
			if len(cleanParts) > 3 {
				unit = cleanParts[3]
			}

			configs[cleanUUID] = TagConfig{
				UUID: cleanUUID,
				DID:  did,
				DNM:  dnm,
				NM:   nm,
				Unit: unit,
			}
		}
	}
	return configs
}

func parseValuesPayload(payloadStr string, configs map[string]TagConfig) map[string]TagValue {
	var jsonStr string

	if strings.HasPrefix(payloadStr, "H4s") || strings.HasPrefix(payloadStr, "H4sI") {
		data, err := base64.StdEncoding.DecodeString(payloadStr)
		if err == nil {
			r, err := gzip.NewReader(bytes.NewReader(data))
			if err == nil {
				decomp, _ := io.ReadAll(r)
				jsonStr = string(decomp)
			}
		}
	}

	if jsonStr == "" {
		jsonStr = payloadStr
	}

	var rawList []map[string]string
	if err := json.Unmarshal([]byte(jsonStr), &rawList); err != nil {
		return nil
	}

	values := make(map[string]TagValue)
	for _, item := range rawList {
		for uuid, valStr := range item {
			cleanUUID := strings.Trim(uuid, "'\" ")
			cfg, exists := configs[cleanUUID]
			if !exists {
				continue
			}

			parts := strings.Split(valStr, ",")
			cleanParts := make([]string, len(parts))
			for i, p := range parts {
				cleanParts[i] = strings.Trim(p, "'\" ")
			}

			val := ""
			if len(cleanParts) > 0 {
				val = cleanParts[0]
			}
			alarmSt := ""
			if len(cleanParts) > 1 {
				alarmSt = cleanParts[1]
			}
			ts := ""
			if len(cleanParts) > 2 {
				ts = cleanParts[2]
			}
			qos := ""
			if len(cleanParts) > 3 {
				qos = cleanParts[3]
			}

			values[cleanUUID] = TagValue{
				UUID:      cleanUUID,
				NM:        cfg.NM,
				ValStr:    val,
				AlarmSt:   alarmSt,
				Timestamp: ts,
				QOS:       qos,
			}
		}
	}
	return values
}
