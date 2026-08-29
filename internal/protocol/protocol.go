package protocol

type Message struct {
	Type            string     `json:"type"`
	ID              string     `json:"id,omitempty"`
	Host            string     `json:"host,omitempty"`
	Session         string     `json:"session,omitempty"`
	Sessions        []string   `json:"sessions,omitempty"`
	SessionInfos    []Session  `json:"session_infos,omitempty"`
	Schedules       []Schedule `json:"schedules,omitempty"`
	Capabilities    []string   `json:"capabilities,omitempty"`
	Data            string     `json:"data,omitempty"`
	Text            string     `json:"text,omitempty"`
	Name            string     `json:"name,omitempty"`
	Command         string     `json:"command,omitempty"`
	Value           string     `json:"value,omitempty"`
	DelaySeconds    int64      `json:"delay_seconds,omitempty"`
	Repeat          int        `json:"repeat,omitempty"`
	IntervalSeconds int64      `json:"interval_seconds,omitempty"`
	Cols            uint16     `json:"cols,omitempty"`
	Rows            uint16     `json:"rows,omitempty"`
	Error           string     `json:"error,omitempty"`
}

type Session struct {
	Host         string `json:"host"`
	Session      string `json:"session"`
	LastActivity int64  `json:"last_activity,omitempty"`
}

type Schedule struct {
	ID              string `json:"id"`
	Host            string `json:"host,omitempty"`
	Session         string `json:"session"`
	Pane            string `json:"pane,omitempty"`
	Text            string `json:"text"`
	CreatedAt       int64  `json:"created_at"`
	FirstAt         int64  `json:"first_at"`
	Repeat          int    `json:"repeat"`
	IntervalSeconds int64  `json:"interval_seconds,omitempty"`
}

type SessionList struct {
	HostCount int        `json:"host_count"`
	Hosts     []string   `json:"hosts"`
	Sessions  []Session  `json:"sessions"`
	Schedules []Schedule `json:"schedules,omitempty"`
}
