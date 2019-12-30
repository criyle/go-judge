module github.com/criyle/go-judge

go 1.13

require (
	github.com/criyle/go-sandbox v0.0.0-20191225131813-d1ed5f0f21dd
	github.com/googollee/go-engine.io v1.4.2
	github.com/googollee/go-socket.io v1.4.2
	github.com/ugorji/go/codec v1.1.7
)

// use modified socket.io with longer timeout
replace github.com/googollee/go-engine.io => github.com/criyle/go-engine.io v1.4.3-0.20191226121441-e9662a4bcdfa

replace github.com/criyle/go-sandbox => ../go-sandbox
