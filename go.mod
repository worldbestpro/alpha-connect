module gitlab.com/alphaticks/alphac

go 1.13

require (
	github.com/AsynkronIT/protoactor-go v0.0.0-20200317173033-c483abfa40e2
	github.com/gogo/protobuf v1.3.1
	github.com/satori/go.uuid v1.2.0
	gitlab.com/alphaticks/gorderbook v0.0.0-20200703081116-690c4dda7a71
	gitlab.com/alphaticks/xchanger v0.0.0-20200717145256-b3a6e2fd0bfa
	gitlab.com/tachikoma.ai/tickpred v0.0.0-20200710184839-b7106d6c9d63 // indirect
)

//replace gitlab.com/alphaticks/xchanger => ../../alphaticks/xchanger
