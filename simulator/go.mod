module github.com/CB2Moon/DJI_RC_Nx_Translator/simulator

go 1.22.4

toolchain go1.24.2

require (
	github.com/CB2Moon/DJI_RC_Nx_Translator v0.0.0-00010101000000-000000000000
	go.bug.st/serial v1.6.4
)

require (
	github.com/creack/goselect v0.1.2 // indirect
	golang.org/x/sys v0.19.0 // indirect
)

replace github.com/CB2Moon/DJI_RC_Nx_Translator => ../
