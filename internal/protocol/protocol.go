package protocol

type Message struct {
	Type            string   `json:"type"`
	ID              string   `json:"id,omitempty"`
	Host            string   `json:"host,omitempty"`
	Session         string   `json:"session,omitempty"`
	Sessions        []string `json:"sessions,omitempty"`
	Data            string   `json:"data,omitempty"`
	Text            string   `json:"text,omitempty"`
	Name            string   `json:"name,omitempty"`
	Command         string   `json:"command,omitempty"`
	Value           string   `json:"value,omitempty"`
	DelaySeconds    int64    `json:"delay_seconds,omitempty"`
	Repeat          int      `json:"repeat,omitempty"`
	IntervalSeconds int64    `json:"interval_seconds,omitempty"`
	Cols            uint16   `json:"cols,omitempty"`
	Rows            uint16   `json:"rows,omitempty"`
	Error           string   `json:"error,omitempty"`
}

type Session struct {
	Host    string `json:"host"`
	Session string `json:"session"`
}

type SessionList struct {
	HostCount int       `json:"host_count"`
	Sessions  []Session `json:"sessions"`
}
