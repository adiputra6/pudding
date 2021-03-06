package main

import (
	"os"

	"github.com/codegangsta/cli"
	"github.com/travis-ci/pudding/lib"
	"github.com/travis-ci/pudding/lib/server"
)

func main() {
	app := cli.NewApp()
	app.Version = lib.VersionString
	app.Flags = []cli.Flag{
		lib.AddrFlag,
		lib.RedisURLFlag,
		cli.StringFlag{
			Name:   "instance-builds-queue-name",
			Value:  "instance-builds",
			EnvVar: "PUDDING_INSTANCE_BUILDS_QUEUE_NAME",
		},
		cli.StringFlag{
			Name:   "instance-terminations-queue-name",
			Value:  "instance-terminations",
			EnvVar: "PUDDING_INSTANCE_TERMINATIONS_QUEUE_NAME",
		},
		cli.StringFlag{
			Name:   "A, auth-token",
			Value:  "swordfish",
			EnvVar: "PUDDING_AUTH_TOKEN",
		},
		lib.SlackHookPathFlag,
		lib.SlackUsernameFlag,
		lib.SlackChannelFlag,
		lib.SlackIconFlag,
		lib.SentryDSNFlag,
		lib.InstanceExpiryFlag,
		lib.ImageExpiryFlag,
		lib.DebugFlag,
	}
	app.Action = runServer

	app.Run(os.Args)
}

func runServer(c *cli.Context) {
	lib.WriteFlagsToEnv(c)

	server.Main(&server.Config{
		Addr:      c.String("addr"),
		AuthToken: c.String("auth-token"),
		Debug:     c.Bool("debug"),

		RedisURL: c.String("redis-url"),

		SlackHookPath:       c.String("slack-hook-path"),
		SlackUsername:       c.String("slack-username"),
		SlackIcon:           c.String("slack-icon"),
		DefaultSlackChannel: c.String("default-slack-channel"),

		SentryDSN: c.String("sentry-dsn"),

		InstanceExpiry: c.Int("instance-expiry"),
		ImageExpiry:    c.Int("image-expiry"),

		QueueNames: map[string]string{
			"instance-builds":       c.String("instance-builds-queue-name"),
			"instance-terminations": c.String("instance-terminations-queue-name"),
		},
	})
}
