module go-worker

go 1.26.2

require (
	conductor-testing/proto v0.0.0
	conductor-testing/sharedlib v0.0.0
	github.com/pmenglund/codex-sdk-go v0.0.0-20260509000142-89b0f6c5c176
	google.golang.org/grpc v1.81.0
)

require (
	golang.org/x/net v0.51.0 // indirect
	golang.org/x/sys v0.42.0 // indirect
	golang.org/x/text v0.34.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260226221140-a57be14db171 // indirect
	google.golang.org/protobuf v1.36.11 // indirect
)

replace conductor-testing/proto => ../../proto

replace conductor-testing/sharedlib => ../../sharedlib
