package main

import (
	"flag"
	"time"
)

type Config struct {
	Listen            string
	DataDir           string
	WebDir            string
	HeartbeatTimeout  int64
	OpenTimeout       time.Duration
	PollInterval      time.Duration
	SnapshotInterval  time.Duration
	HeartbeatInterval time.Duration
}

func LoadConfig(args []string) Config {
	flags := flag.NewFlagSet("afc", flag.ContinueOnError)
	listen := flags.String("listen", "127.0.0.1:8080", "http listen address")
	dataDir := flags.String("data", "./data", "data directory")
	webDir := flags.String("web", "./web", "web assets directory")
	heartbeatTimeout := flags.Int64("heartbeat-timeout", 60, "heartbeat timeout in seconds")
	openTimeout := flags.Duration("open-timeout", 3*time.Second, "sensor confirmation timeout")
	pollInterval := flags.Duration("poll-interval", 5*time.Second, "gate poll interval")
	snapshotInterval := flags.Duration("snapshot-interval", 30*time.Second, "passage snapshot interval")
	heartbeatInterval := flags.Duration("heartbeat-interval", 5*time.Second, "heartbeat sweep interval")
	_ = flags.Parse(args)
	return Config{
		Listen:            *listen,
		DataDir:           *dataDir,
		WebDir:            *webDir,
		HeartbeatTimeout:  *heartbeatTimeout,
		OpenTimeout:       *openTimeout,
		PollInterval:      *pollInterval,
		SnapshotInterval:  *snapshotInterval,
		HeartbeatInterval: *heartbeatInterval,
	}
}
