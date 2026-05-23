package handlers

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"strings"

	"corp-vpn-bot/internal/config"
	"corp-vpn-bot/internal/db"
	"corp-vpn-bot/internal/remnawave"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

type Handlers struct {
	cfg    *config.Config
	db     *db.DB
	rw     *remnawave.Client
	log    *slog.Logger
	states *stateStore
}

func New(cfg *config.Config, database *db.DB, rw *remnawave.Client, log *slog.Logger) *Handlers {
	return &Handlers{
		cfg:    cfg,
		db:     database,
		rw:     rw,
		log:    log,
		states: newStateStore(),
	}
}

// Register регистрирует все обработчики на боте.
func (h *Handlers) Register(b *bot.Bot) {
	// Используем регулярки, чтобы /add не перехватывал /addlist
	b.RegisterHandlerRegexp(bot.HandlerTypeMessageText,
		regexp.MustCompile(`(?i)^/start(@\w+)?(\s|$)`), h.OnStart)
	b.RegisterHandlerRegexp(bot.HandlerTypeMessageText,
		regexp.MustCompile(`(?i)^/help(@\w+)?(\s|$)`), h.OnHelp)
	b.RegisterHandlerRegexp(bot.HandlerTypeMessageText,
		regexp.MustCompile(`(?i)^/list(@\w+)?(\s|$)`), h.OnList)
	b.RegisterHandlerRegexp(bot.HandlerTypeMessageText,
		regexp.MustCompile(`(?i)^/addlist(@\w+)?(\s|$)`), h.OnAddList)
	b.RegisterHandlerRegexp(bot.HandlerTypeMessageText,
		regexp.MustCompile(`(?i)^/add(@\w+)?(\s|$)`), h.OnAdd)
	b.RegisterHandlerRegexp(bot.HandlerTypeMessageText,
		regexp.MustCompile(`(?i)^/del(@\w+)?(\s|$)`), h.OnDel)
	b.RegisterHandlerRegexp(bot.HandlerTypeMessageText,
		regexp.MustCompile(`(?i)^/cancel(@\w+)?(\s|$)`), h.OnCancel)
}

// FallbackHandler — для произвольного текста (нужен для приёма списка после /addlist).
func (h *Handlers) Fallback(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.Message == nil || update.Message.From == nil {
		return
	}
	uid := update.Message.From.ID

	if !h.cfg.IsAdmin(uid) {
		return
	}
	st, ok := h.states.get(uid)
	if !ok {
		return
	}
	if st == stateAwaitList {
		h.states.clear(uid)
		h.processList(ctx, b, update.Message)
	}
}

// --- /start (для всех) ---

func (h *Handlers) OnStart(ctx context.Context, b *bot.Bot, u *models.Update) {
	m := u.Message
	if m == nil || m.From == nil {
		return
	}

	uname := normalizeUsername(m.From.Username)
	if uname == "" {
		reply(ctx, b, m, "У вашего Telegram-аккаунта не задан username. "+
			"Установите его в настройках Telegram и попробуйте снова.")
		return
	}

	emp, err := h.db.GetByUsername(ctx, uname)
	if err != nil {
		h.log.Error("get by username", "err", err)
		reply(ctx, b, m, "Внутренняя ошибка. Попробуйте позже.")
		return
	}
	if emp == nil {
		reply(ctx, b, m,
			"Пользователь не создан. Обратитесь к администратору.")
		return
	}

	// Первое появление: запомним telegram_id для аудита
	if emp.TelegramID == nil {
		if err := h.db.MarkActivated(ctx, uname, m.From.ID); err != nil {
			h.log.Warn("mark activated", "err", err)
		}
	}

	reply(ctx, b, m, fmt.Sprintf(
		"Ваш VPN-доступ:\n\n<code>%s</code>\n\n"+
			"Скопируйте ссылку и вставьте в клиент: v2rayTun, Hiddify, Happ или FoXray.",
		escapeHTML(emp.SubscriptionURL),
	))
}

// --- /help ---

func (h *Handlers) OnHelp(ctx context.Context, b *bot.Bot, u *models.Update) {
	m := u.Message
	if m == nil || m.From == nil {
		return
	}
	if h.cfg.IsAdmin(m.From.ID) {
		reply(ctx, b, m, strings.Join([]string{
			"<b>Админ-команды:</b>",
			"/list — список сотрудников",
			"/add username [username2 ...] — добавить одного или несколько",
			"/addlist — добавить списком (по одному на строку)",
			"/del username — удалить",
			"/cancel — отменить ожидание ввода",
			"",
			"<b>Для сотрудников:</b>",
			"/start — получить ссылку подписки",
		}, "\n"))
		return
	}
	reply(ctx, b, m, "Команды: /start — получить ссылку на VPN.")
}

// --- /list (admin) ---

func (h *Handlers) OnList(ctx context.Context, b *bot.Bot, u *models.Update) {
	m := u.Message
	if m == nil || m.From == nil || !h.cfg.IsAdmin(m.From.ID) {
		return
	}

	emps, err := h.db.List(ctx, 200)
	if err != nil {
		h.log.Error("list", "err", err)
		reply(ctx, b, m, "Ошибка чтения базы.")
		return
	}
	if len(emps) == 0 {
		reply(ctx, b, m, "Сотрудников нет.")
		return
	}

	total, _ := h.db.Count(ctx)
	var sb strings.Builder
	fmt.Fprintf(&sb, "<b>Сотрудники (%d):</b>\n\n", total)
	for _, e := range emps {
		status := "🕓 не активирован"
		if e.ActivatedAt != nil {
			status = "✅ активирован " + e.ActivatedAt.Format("02.01.2006")
		}
		fmt.Fprintf(&sb, "• <code>%s</code> — %s\n", escapeHTML(e.Username), status)
	}
	if total > 200 {
		fmt.Fprintf(&sb, "\n<i>Показаны первые 200 из %d.</i>", total)
	}

	// Telegram message limit ~4096
	sendChunked(ctx, b, m.Chat.ID, sb.String())
}

// --- /add (admin) ---

// extractArgs возвращает то, что идёт после команды, игнорируя саму команду
// и опциональный суффикс @botname.
func extractArgs(text, cmd string) string {
	// /cmd args  или  /cmd@bot args
	rest := strings.TrimPrefix(text, cmd)
	if strings.HasPrefix(rest, "@") {
		if i := strings.IndexAny(rest, " \t\n"); i >= 0 {
			rest = rest[i:]
		} else {
			rest = ""
		}
	}
	return strings.TrimSpace(rest)
}

var usernameRe = regexp.MustCompile(`^[A-Za-z0-9_]{3,32}$`)

func (h *Handlers) OnAdd(ctx context.Context, b *bot.Bot, u *models.Update) {
	m := u.Message
	if m == nil || m.From == nil || !h.cfg.IsAdmin(m.From.ID) {
		return
	}

	args := strings.Fields(extractArgs(m.Text, "/add"))
	if len(args) == 0 {
		reply(ctx, b, m, "Использование: <code>/add username [username2 ...]</code>")
		return
	}

	h.addUsernames(ctx, b, m, args)
}

// --- /addlist (admin) ---

func (h *Handlers) OnAddList(ctx context.Context, b *bot.Bot, u *models.Update) {
	m := u.Message
	if m == nil || m.From == nil || !h.cfg.IsAdmin(m.From.ID) {
		return
	}

	// Если в той же команде сразу указали список — обработаем сразу
	rest := extractArgs(m.Text, "/addlist")
	if rest != "" {
		h.addUsernamesFromText(ctx, b, m, rest)
		return
	}

	h.states.set(m.From.ID, stateAwaitList)
	reply(ctx, b, m,
		"Пришлите список Telegram username — <b>по одному на строку</b>, без <code>@</code>.\n\n"+
			"Пример:\n<code>ivanov\npetrov\nsidorov</code>\n\n"+
			"Отмена: /cancel",
	)
}

func (h *Handlers) processList(ctx context.Context, b *bot.Bot, m *models.Message) {
	h.addUsernamesFromText(ctx, b, m, m.Text)
}

func (h *Handlers) addUsernamesFromText(ctx context.Context, b *bot.Bot, m *models.Message, text string) {
	var names []string
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Разрешаем как с @, так и без — нормализуем дальше
		for _, p := range strings.Fields(line) {
			names = append(names, p)
		}
	}
	if len(names) == 0 {
		reply(ctx, b, m, "Пустой список.")
		return
	}
	h.addUsernames(ctx, b, m, names)
}

func (h *Handlers) addUsernames(ctx context.Context, b *bot.Bot, m *models.Message, raw []string) {
	var (
		added   []string
		exists  []string
		invalid []string
		failed  []string
	)

	for _, u := range raw {
		uname := normalizeUsername(u)
		if !usernameRe.MatchString(uname) {
			invalid = append(invalid, u)
			continue
		}

		existing, err := h.db.GetByUsername(ctx, uname)
		if err != nil {
			h.log.Error("get", "u", uname, "err", err)
			failed = append(failed, uname+" (db error)")
			continue
		}
		if existing != nil {
			exists = append(exists, uname)
			continue
		}

		user, err := h.rw.CreateUser(ctx, uname,
			h.cfg.SubDays, h.cfg.SubDevices, h.cfg.SubTrafficGB)
		if err != nil {
			h.log.Error("remnawave create", "u", uname, "err", err)
			failed = append(failed, fmt.Sprintf("%s (%s)", uname, truncate(err.Error(), 80)))
			continue
		}

		err = h.db.AddEmployee(ctx, db.Employee{
			Username:        uname,
			RemnawaveUUID:   user.UUID,
			SubscriptionURL: user.SubscriptionURL,
			CreatedBy:       m.From.ID,
		})
		if err != nil {
			h.log.Error("db add", "u", uname, "err", err)
			// Откат: удалим из Remnawave, чтобы не плодить сирот
			if delErr := h.rw.DeleteUser(ctx, user.UUID); delErr != nil {
				h.log.Error("rollback rw delete", "err", delErr)
			}
			failed = append(failed, uname+" (db save)")
			continue
		}
		added = append(added, uname)
	}

	var sb strings.Builder
	if len(added) > 0 {
		fmt.Fprintf(&sb, "✅ Добавлены (%d):\n", len(added))
		for _, u := range added {
			fmt.Fprintf(&sb, "  • <code>%s</code>\n", escapeHTML(u))
		}
	}
	if len(exists) > 0 {
		fmt.Fprintf(&sb, "\nℹ️ Уже были (%d):\n", len(exists))
		for _, u := range exists {
			fmt.Fprintf(&sb, "  • <code>%s</code>\n", escapeHTML(u))
		}
	}
	if len(invalid) > 0 {
		fmt.Fprintf(&sb, "\n⚠️ Некорректные (%d):\n", len(invalid))
		for _, u := range invalid {
			fmt.Fprintf(&sb, "  • <code>%s</code>\n", escapeHTML(u))
		}
	}
	if len(failed) > 0 {
		fmt.Fprintf(&sb, "\n❌ Ошибки (%d):\n", len(failed))
		for _, u := range failed {
			fmt.Fprintf(&sb, "  • <code>%s</code>\n", escapeHTML(u))
		}
	}
	if sb.Len() == 0 {
		sb.WriteString("Ничего не сделано.")
	}
	sendChunked(ctx, b, m.Chat.ID, sb.String())
}

// --- /del (admin) ---

func (h *Handlers) OnDel(ctx context.Context, b *bot.Bot, u *models.Update) {
	m := u.Message
	if m == nil || m.From == nil || !h.cfg.IsAdmin(m.From.ID) {
		return
	}
	args := strings.Fields(extractArgs(m.Text, "/del"))
	if len(args) == 0 {
		reply(ctx, b, m, "Использование: <code>/del username</code>")
		return
	}

	uname := normalizeUsername(args[0])
	if !usernameRe.MatchString(uname) {
		reply(ctx, b, m, "Некорректный username.")
		return
	}

	emp, err := h.db.GetByUsername(ctx, uname)
	if err != nil {
		h.log.Error("get for del", "err", err)
		reply(ctx, b, m, "Ошибка БД.")
		return
	}
	if emp == nil {
		reply(ctx, b, m, "Такого сотрудника нет в базе.")
		return
	}

	if err := h.rw.DeleteUser(ctx, emp.RemnawaveUUID); err != nil {
		h.log.Error("rw delete", "err", err)
		reply(ctx, b, m, fmt.Sprintf("Ошибка Remnawave: <code>%s</code>",
			escapeHTML(truncate(err.Error(), 200))))
		return
	}

	if _, _, err := h.db.Delete(ctx, uname); err != nil {
		h.log.Error("db delete", "err", err)
		reply(ctx, b, m, "Удалено в Remnawave, но не в БД. Сообщите разработчику.")
		return
	}

	reply(ctx, b, m, fmt.Sprintf("Сотрудник <code>%s</code> удалён.", escapeHTML(uname)))
}

// --- /cancel ---

func (h *Handlers) OnCancel(ctx context.Context, b *bot.Bot, u *models.Update) {
	m := u.Message
	if m == nil || m.From == nil {
		return
	}
	if _, ok := h.states.get(m.From.ID); ok {
		h.states.clear(m.From.ID)
		reply(ctx, b, m, "Окей, отменил.")
		return
	}
	reply(ctx, b, m, "Нечего отменять.")
}

// --- helpers ---

func normalizeUsername(s string) string {
	return strings.ToLower(strings.TrimPrefix(strings.TrimSpace(s), "@"))
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func escapeHTML(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")
	return r.Replace(s)
}

func reply(ctx context.Context, b *bot.Bot, m *models.Message, text string) {
	_, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    m.Chat.ID,
		Text:      text,
		ParseMode: models.ParseModeHTML,
	})
	if err != nil {
		// Без ParseMode на случай ошибок разметки
		_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: m.Chat.ID,
			Text:   text,
		})
	}
}

func sendChunked(ctx context.Context, b *bot.Bot, chatID int64, text string) {
	const maxLen = 3800
	for i := 0; i < len(text); i += maxLen {
		end := i + maxLen
		if end > len(text) {
			end = len(text)
		}
		_, _ = b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID:    chatID,
			Text:      text[i:end],
			ParseMode: models.ParseModeHTML,
		})
	}
}
