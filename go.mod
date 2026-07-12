module github.com/sakaisatoru/go_radio_br_zero

go 1.25.5

replace local.packages/aqm0802a => ./aqm0802a

replace local.packages/volume => ./volume

require (
	github.com/davecheney/i2c v0.0.0-20140823063045-caf08501bef2
	github.com/sakaisatoru/go_mpvradio/netradio v0.0.0-20260712142908-a5600720cb47
	github.com/sakaisatoru/go_radio_raspi/mpvctl v0.0.0-20260711065057-a1f510d3716d
	github.com/sakaisatoru/go_radio_raspi/rotaryencoder v0.0.0-20260704043411-6d5c42f2b65b
	github.com/stianeikeland/go-rpio/v4 v4.6.0
	local.packages/aqm0802a v0.0.0-00010101000000-000000000000
	local.packages/volume v0.0.0-00010101000000-000000000000
)

require (
	github.com/carlmjohnson/requests v0.25.1 // indirect
	golang.org/x/net v0.38.0 // indirect
)
