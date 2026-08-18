package sfms

import "time"

// Result represents the status result of an API response.
type Result struct {
	Flag    bool   `json:"Flag"`
	Message string `json:"Message"`
}

// FeederInfo represents data for an individual feeder returned by GETCO SFMS REST API.
type FeederInfo struct {
	FID         int    `json:"fid"`
	Cat         int    `json:"cat"`
	Type        string `json:"type"`
	Name        string `json:"name"`
	ISACT       bool   `json:"ISACT"`
	BMUSerialNo string `json:"BMU_Serial_No"`
	BMUISACT    bool   `json:"BMU_ISACT"`
	FdrCode     string `json:"Fdr_Code"`
	Device      string `json:"Device"`
	Seq         int    `json:"Seq"`
	EqupNo      int    `json:"Equp_no"`
}

// Substation represents a substation containing multiple feeders.
type Substation struct {
	SSID       int          `json:"SSID"`
	Name       string       `json:"NAME"`
	FeederInfo []FeederInfo `json:"FeederInfo"`
}

// APIResponse is the top-level payload returned by Substation_feederInfo.
type APIResponse struct {
	Result Result       `json:"Result"`
	Data   []Substation `json:"Data"`
}

// DeviceGroup represents a telemetry mapping group returned by /Device/GetDevicemappedGroup.
type DeviceGroup struct {
	GrpID string `json:"grpId"`
	Name  string `json:"nm"`
	Typ   int    `json:"typ"`
	Desc  string `json:"desc"`
	Did   string `json:"Did"`
}

// DeviceGroupResponse represents the response payload for /Device/GetDevicemappedGroup.
type DeviceGroupResponse struct {
	Result Result        `json:"Result"`
	Data   []DeviceGroup `json:"Data"`
}

// RequestPayload represents the POST payload sent to GETCO SFMS API.
type RequestPayload struct {
	Depid string `json:"Depid"`
	DivID string `json:"Div_Id"`
}

// LoginRequest represents the authentication payload for GETCO SFMS API.
type LoginRequest struct {
	Username string `json:"Username"`
	Password string `json:"Password"`
	CsComp   string `json:"CsComp"`
	OTP      string `json:"OTP"`
}

// LoginResponse represents the login API response payload.
type LoginResponse struct {
	AccessToken string `json:"access_token"`
	Result      Result `json:"Result"`
}

// LiveBreakerState tracks the real-time circuit breaker status decoded from MQTT telemetry.
type LiveBreakerState struct {
	DeviceID  string
	Seq       int
	CBON      int
	CBOFF     int
	Status    string // "CLOSED", "OPEN", "UNKNOWN", "ERROR"
	Timestamp string
	UpdatedAt time.Time
}

// TagConfig represents metadata for a telemetry tag.
type TagConfig struct {
	UUID string
	DID  string
	DNM  string
	NM   string
	Unit string
}

// TagValue represents a live value received for a tag.
type TagValue struct {
	UUID      string
	NM        string
	ValStr    string
	AlarmSt   string
	Timestamp string
	QOS       string
}

// FeederState tracks the current runtime status of a feeder in memory.
type FeederState struct {
	FID              int
	Name             string
	Category         int
	CategoryName     string
	Is24x7           bool
	ScheduleStart    string
	ScheduleEnd      string
	SubstationID     int
	SubstationName   string
	BMUSerialNo      string
	FdrCode          string
	Device           string
	Seq              int
	IsActive         bool
	BMUIsActive      bool
	CBON             int
	CBOFF            int
	HasTelemetry     bool
	BreakerStatus    string
	IsOnline         bool
	InterruptedSince *time.Time
	HasStartedToday  bool
	LastWindowDate   string
}
