package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"strata-proof/internal/application"
	"strata-proof/internal/evidence"
	"strata-proof/internal/httpui"
	"strata-proof/internal/store"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		log.Printf("strata-proof 退出：%v", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	cfg, err := parseConfig(args)
	if err != nil {
		return err
	}
	database := cfg.database
	if cfg.selfcheck {
		database = ":memory:"
	}
	repository, err := store.Open(database)
	if err != nil {
		return err
	}
	defer repository.Close()
	analyzer := evidence.NewAnalyzer()
	issuer := evidence.NewCredentialIssuer("strata-proof-local-credential-v1")
	service := application.NewService(repository, analyzer, issuer)
	handler := httpui.NewHandler(service).Routes()
	if cfg.selfcheck {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		return httpui.SelfCheck(ctx, cfg.addr, handler)
	}
	server := &http.Server{Addr: cfg.addr, Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second}
	stop, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		log.Printf("地层证据工作台正在 http://%s/workbench 提供服务", cfg.addr)
		err := server.ListenAndServe()
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		done <- err
	}()
	select {
	case err := <-done:
		return err
	case <-stop.Done():
		ctx, shutdown := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdown()
		if err := server.Shutdown(ctx); err != nil {
			return fmt.Errorf("关闭 HTTP 服务: %w", err)
		}
		return <-done
	}
}
