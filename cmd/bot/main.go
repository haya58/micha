package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	dbot "dis-llm-bot/internal/discord"
	"dis-llm-bot/internal/llm"
	"dis-llm-bot/internal/scheduler"

	"dis-llm-bot/internal/config"
	"dis-llm-bot/internal/persona"
	"dis-llm-bot/internal/store"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	ctx := context.Background()
	st, err := store.Connect(ctx, cfg.Database.URL, cfg.Models.EmbeddingDim)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer st.Close()
	log.Printf("db connected and migrated (messages, memories, tool_runs)")

	personaText, err := persona.Load(cfg.Persona.Path)
	if err != nil {
		log.Fatalf("persona: %v", err)
	}
	endpoints := make(map[string]llm.Endpoint, len(cfg.Endpoints))
	for name, ep := range cfg.Endpoints {
		endpoints[name] = llm.Endpoint{
			BaseURL:      ep.BaseURL,
			APIKey:       ep.APIKey,
			ExtraHeaders: ep.ExtraHeaders,
		}
	}
	lc, err := llm.New(endpoints, cfg.Models.Chat, cfg.Models.Embedding)
	if err != nil {
		log.Fatalf("llm: %v", err)
	}

	b, err := dbot.New(cfg, st, lc, personaText)
	if err != nil {
		log.Fatalf("discord: %v", err)
	}
	if err := b.Open(); err != nil {
		log.Fatalf("discord open: %v", err)
	}
	defer b.Close()
	log.Printf("bot started guild=%s categories=%v chat=%s embedding=%s", cfg.Discord.GuildID, cfg.Discord.CategoryIDs, cfg.Models.Chat, cfg.Models.Embedding)

	// health endpoint for k8s/docker
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		if err := st.Pool.Ping(r.Context()); err != nil {
			w.WriteHeader(500)
			_, _ = w.Write([]byte("db ng"))
			return
		}
		_, _ = w.Write([]byte("ok"))
	})
	go func() { _ = http.ListenAndServe(cfg.Server.HealthAddr, mux) }()

	stop := make(chan struct{})
	defer close(stop)
	go scheduler.RandomTalk(cfg, b, stop)
	go scheduler.PollReadonly(cfg, b, stop)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	log.Println("shutting down")
}
