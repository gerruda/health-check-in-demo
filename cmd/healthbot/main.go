// Command healthbot — Telegram-бот персонального тренера (health-проект).

// Этап M0 (см. бот/ТЗ_телеграм_бот.md): базовая основа — конфигурация,
// логирование, long polling, allowlist по chat_id и обработчик /help.
// Бизнес-логика (чек-ины, тренировки, питание, замеры, статьи) добавляется
// следующими этапами по ТЗ.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"healthbot/internal/app"
	"healthbot/internal/intake"
	"healthbot/internal/webhookcert"
)

func main() {
	dryRun := flag.Bool("dry-run", false, "самопроверка: конфиг и обработчик /help без подключения к Telegram")
	ask := flag.String("ask", "", "задать вопрос LLM и напечатать ответ (без Telegram, смоук модели и инструментов)")
	image := flag.String("image", "", "распознать скриншот(ы) через vision (пути через запятую, без Telegram)")
	checkin := flag.String("checkin", "", "распознать текстовый чек-ин без Telegram (смоук)")
	genWebhookCert := flag.Bool("gen-webhook-cert", false, "сгенерировать self-signed сертификат для webhook и выйти")
	webhookCertOut := flag.String("webhook-cert-out", "", "путь публичного сертификата (PEM)")
	webhookKeyOut := flag.String("webhook-key-out", "", "путь приватного ключа (PEM)")
	webhookHost := flag.String("webhook-host", "", "IP или DNS для сертификата (SAN)")
	webhookCertDays := flag.Int("webhook-cert-days", 3650, "срок действия сертификата, дней")
	webhookCertForce := flag.Bool("webhook-cert-force", false, "перезаписать существующие файлы")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if *genWebhookCert {
		info, err := webhookcert.Generate(webhookcert.Options{
			CertPath: *webhookCertOut,
			KeyPath:  *webhookKeyOut,
			Host:     *webhookHost,
			Days:     *webhookCertDays,
			Force:    *webhookCertForce,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "healthbot: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("cert: %s\nkey: %s\nhost: %s\nnot_after: %s\nfingerprint_sha256: %s\n",
			info.CertPath, info.KeyPath, info.CommonName, info.NotAfter.Format(time.RFC3339), info.Fingerprint)
		return
	}

	if *image != "" {
		paths := strings.Split(*image, ",")
		for index := range paths {
			paths[index] = strings.TrimSpace(paths[index])
		}
		extracted, err := app.ScanImage(ctx, paths)
		if err != nil {
			fmt.Fprintf(os.Stderr, "healthbot: %v\n", err)
			os.Exit(1)
		}
		raw, err := json.MarshalIndent(extracted, "", "  ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "healthbot: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(string(raw))
		fmt.Println()
		fmt.Println(intake.FormatReply(extracted))
		return
	}

	if *checkin != "" {
		extracted, err := app.ScanCheckin(ctx, *checkin)
		if err != nil {
			fmt.Fprintf(os.Stderr, "healthbot: %v\n", err)
			os.Exit(1)
		}
		raw, err := json.MarshalIndent(extracted, "", "  ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "healthbot: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(string(raw))
		fmt.Println()
		fmt.Println(intake.FormatReply(extracted))
		return
	}

	if *ask != "" {
		answer, err := app.Ask(ctx, *ask)
		if err != nil {
			fmt.Fprintf(os.Stderr, "healthbot: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(answer)
		return
	}

	if err := app.Run(ctx, *dryRun); err != nil {
		fmt.Fprintf(os.Stderr, "healthbot: %v\n", err)
		os.Exit(1)
	}
}
