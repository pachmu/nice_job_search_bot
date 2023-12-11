package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"syscall"

	"github.com/pachmu/nice_job_search_bot/config"
	"github.com/pachmu/nice_job_search_bot/internal/bot"
	"github.com/pachmu/nice_job_search_bot/internal/db"

	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
)

var configPath = flag.String("config", "./config/config.yaml", "Path to config file")

func main() {
	flag.Parse()
	conf, err := config.GetConfig(*configPath)
	if err != nil {
		logrus.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	errGr, ctx := errgroup.WithContext(ctx)
	sqliteDB, err := db.NewSQLiteDB(conf.Sqlite.Datasource)
	if err != nil {
		logrus.Fatal(err)
	}
	handler := bot.NewMessageHandler(conf.Bot.ChatID, sqliteDB)
	bt, err := bot.NewTelegramBot(conf.Bot.Token, handler)
	if err != nil {
		logrus.Fatal(err)
	}
	errGr.Go(func() error {
		quitCh := make(chan os.Signal, 1)
		signal.Notify(quitCh, os.Kill, os.Interrupt, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
		select {
		case <-quitCh:
		case <-ctx.Done():
		}

		cancel()
		return nil
	})

	errGr.Go(func() error {
		err := bt.Run(ctx)
		if err != nil {
			return err
		}
		return nil
	})
	logrus.Info("Bot started")

	err = errGr.Wait()
	if err != nil {
		logrus.Fatal(err)
	}
	logrus.Info("Process terminated")
}
