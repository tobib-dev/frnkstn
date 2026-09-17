module github.com/tobib-dev/frnkstn

go 1.25.8

require (
	charm.land/bubbles/v2 v2.1.0
	charm.land/bubbletea/v2 v2.0.2
	charm.land/lipgloss/v2 v2.0.2
	charm.land/log/v2 v2.0.0
	charm.land/wish/v2 v2.0.0
	github.com/charmbracelet/ssh v0.0.0-20250826160808-ebfa259c7309
	github.com/gocql/gocql v1.7.0
	github.com/joho/godotenv v1.5.1
	github.com/scylladb/gocqlx/v3 v3.0.4
	github.com/tobib-dev/frnkstn-proto v0.0.0-20260916200540-f93d084742a7
	google.golang.org/grpc v1.79.3
)

require google.golang.org/protobuf v1.36.11 // indirect

require (
	github.com/anmitsu/go-shlex v0.0.0-20200514113438-38f4b401e2be // indirect
	github.com/atotto/clipboard v0.1.4 // indirect
	github.com/charmbracelet/colorprofile v0.4.2 // indirect
	github.com/charmbracelet/keygen v0.5.4 // indirect
	github.com/charmbracelet/ultraviolet v0.0.0-20260205113103-524a6607adb8 // indirect
	github.com/charmbracelet/x/ansi v0.11.6 // indirect
	github.com/charmbracelet/x/conpty v0.1.1 // indirect
	github.com/charmbracelet/x/errors v0.0.0-20251110184232-6ab307057ac7 // indirect
	github.com/charmbracelet/x/term v0.2.2 // indirect
	github.com/charmbracelet/x/termios v0.1.1 // indirect
	github.com/charmbracelet/x/windows v0.2.2 // indirect
	github.com/clipperhouse/displaywidth v0.11.0 // indirect
	github.com/clipperhouse/uax29/v2 v2.7.0 // indirect
	github.com/creack/pty v1.1.24 // indirect
	github.com/go-logfmt/logfmt v0.6.1 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/klauspost/compress v1.18.1 // indirect
	github.com/lucasb-eyer/go-colorful v1.3.0 // indirect
	github.com/mattn/go-runewidth v0.0.21 // indirect
	github.com/muesli/cancelreader v0.2.2 // indirect
	github.com/rivo/uniseg v0.4.7 // indirect
	github.com/sahilm/fuzzy v0.1.1 // indirect
	github.com/scylladb/go-reflectx v1.0.1 // indirect
	github.com/xo/terminfo v0.0.0-20220910002029-abceb7e1c41e // indirect
	golang.org/x/crypto v0.48.0 // indirect
	golang.org/x/exp v0.0.0-20251023183803-a4bb9ffd2546 // indirect
	golang.org/x/net v0.49.0 // indirect
	golang.org/x/sync v0.20.0 // indirect
	golang.org/x/sys v0.42.0 // indirect
	golang.org/x/text v0.34.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20251202230838-ff82c1b0f217 // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
)

// Use the latest version of scylladb/gocql; check for updates at https://github.com/scylladb/gocql/releases
replace github.com/gocql/gocql => github.com/scylladb/gocql v1.16.0
