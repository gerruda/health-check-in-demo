package app

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/mymmrac/telego"

	"healthbot/internal/logging"
)

// telegoLogger — адаптер telego.Logger к slog: сообщения telego получают
// формат «[telego:api] ...» и русский префикс.
//
// Важно: telego подставляет токен бота в текст ошибок (URL запроса), а его
// штатный логгер скрывает токен только у себя (DefaultLoggerTokenReplacement).
// При своём логгере маскировать токен обязательно самим — иначе утечка (NFR-1).
type telegoLogger struct {
	log      *slog.Logger
	replacer *strings.Replacer
}

func newTelegoLogger(log *slog.Logger, token string) telegoLogger {
	replacer := strings.NewReplacer()
	if token != "" {
		replacer = strings.NewReplacer(token, telego.DefaultLoggerTokenReplacement)
	}
	return telegoLogger{log: log, replacer: replacer}
}

// Debugf реализует telego.Logger. Debug-уровень скрыт (в нём бывает токен).
func (l telegoLogger) Debugf(format string, args ...any) {
	l.log.Debug(logging.Msg("telego:api", "debug: "+l.replacer.Replace(fmt.Sprintf(format, args...))))
}

// Errorf реализует telego.Logger. Отмена long polling при остановке — штатная,
// поэтому пишется на debug, а не как ошибка.
func (l telegoLogger) Errorf(format string, args ...any) {
	text := l.replacer.Replace(fmt.Sprintf(format, args...))
	if strings.Contains(text, "context canceled") {
		l.log.Debug(logging.Msg("telego:api", "long polling остановлен: "+text))
		return
	}
	l.log.Error(logging.Msg("telego:api", "ошибка Telegram API: "+text))
}
