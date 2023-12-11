package bot

import (
	"context"
	"encoding/json"
	"fmt"
	"runtime/debug"
	"strings"
	"time"

	"github.com/pachmu/nice_job_search_bot/internal/worker"

	"github.com/go-telegram-bot-api/telegram-bot-api"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// TelegramBot represents bot api.
type TelegramBot interface {
	Run(ctx context.Context) error
}

// NewTelegramBot returns telegram api compatible struct.
func NewTelegramBot(token string, handler *MessageHandler) (TelegramBot, error) {
	b, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return &bot{
		handler: handler,
		bot:     b,
	}, nil
}

const (
	actionStart = "/start"
	actionStop  = "/stop"
)

const sexTypesCount = 6

type botActions map[string]func(ctx context.Context, m *tgbotapi.Message, chatParams []string) (tgbotapi.Chattable, error)
type botCallbacks map[string]func(ctx context.Context, query *tgbotapi.CallbackQuery, args []string) (tgbotapi.Chattable, error)

type bot struct {
	handler *MessageHandler
	bot     *tgbotapi.BotAPI
}

func (b *bot) Run(ctx context.Context) error {
	err := b.handler.init(b.bot)
	if err != nil {
		return err
	}
	b.bot.Debug = false

	logrus.Infof("Authorized on account %s", b.bot.Self.UserName)

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates, err := b.bot.GetUpdatesChan(u)
	if err != nil {
		return errors.WithStack(err)
	}

	// Do not handle a large backlog of old messages
	time.Sleep(time.Millisecond * 500)
	updates.Clear()

	for {
		var update tgbotapi.Update
		select {
		case update = <-updates:
			err := b.handler.handle(ctx, update)
			if err != nil {
				logrus.Error(err)
			}
		case <-ctx.Done():
			return nil
		}
	}
}

// NewMessageHandler returns MessageHandler.
func NewMessageHandler(chatID int64, db BotDB) *MessageHandler {
	return &MessageHandler{
		chatID: chatID,
		db:     db,
	}
}

// MessageHandler represents bot message handling functionality.
type MessageHandler struct {
	api       *tgbotapi.BotAPI
	chatID    int64
	actions   botActions
	callbacks botCallbacks
	db        BotDB

	jobSearchCancel context.CancelFunc
}

func (h *MessageHandler) init(api *tgbotapi.BotAPI) error {
	h.api = api

	h.actions = botActions{
		actionStart: func(ctx context.Context, m *tgbotapi.Message, params []string) (tgbotapi.Chattable, error) {
			resp := h.getReplyText(m, "Hey! You can start a job search pressing the button.")
			resp.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
				[]tgbotapi.InlineKeyboardButton{
					tgbotapi.NewInlineKeyboardButtonData("Start a job search!", "start_job_search"),
				},
			)
			return resp, nil
		},
		actionStop: func(ctx context.Context, m *tgbotapi.Message, params []string) (tgbotapi.Chattable, error) {
			h.jobSearchCancel()
			return h.getReplyText(m, "Your job search stopped."), nil
		},
	}

	h.callbacks = botCallbacks{
		"start_job_search": func(ctx context.Context, query *tgbotapi.CallbackQuery, args []string) (tgbotapi.Chattable, error) {
			return h.JobSearchCallback(ctx), nil
		},
	}

	return nil
}

func (h *MessageHandler) handle(ctx context.Context, upd tgbotapi.Update) error {
	defer func() {
		if err := recover(); err != nil {
			logrus.Error(err, string(debug.Stack()))
		}
	}()

	id := getChat(upd)
	err := h.authorize(id)
	if err != nil {
		return err
	}
	logrus.Infof("Message from chat [%d]", id)
	j, err := json.Marshal(upd)
	if err != nil {
		return err
	}
	logrus.Infof("Message [%s]", j)

	var resp tgbotapi.Chattable
	switch {
	case upd.Message != nil:
		resp, err = h.handleActions(ctx, upd.Message)
	case upd.CallbackQuery != nil:
		resp, err = h.handleCallback(ctx, upd.CallbackQuery)
	default:
		return nil
	}
	if err != nil {
		return err
	}
	if resp != nil {
		_, err = h.api.Send(resp)
		if err != nil {
			return errors.WithStack(err)
		}
	}

	return nil
}

func (h *MessageHandler) getReplyText(m *tgbotapi.Message, txt string) *tgbotapi.MessageConfig {
	resp := tgbotapi.NewMessage(m.Chat.ID, txt)
	return &resp
}

func (h *MessageHandler) handleActions(ctx context.Context, msg *tgbotapi.Message) (tgbotapi.Chattable, error) {
	logrus.Infof("Message [%+v]", msg)

	words := strings.Split(msg.Text, " ")
	if len(words) == 0 {
		return h.getReplyText(msg, "Unknown command"), nil
	}
	cmd, ok := h.actions[words[0]]
	if !ok {
		return nil, nil
	}

	resp, err := cmd(ctx, msg, words[1:])
	if err != nil {
		return nil, err
	}

	return resp, nil
}

func (h *MessageHandler) handleCallback(ctx context.Context, query *tgbotapi.CallbackQuery) (tgbotapi.Chattable, error) {
	logrus.Infof("Callback [%+v]", query.Data)
	if len(query.Data) == 0 {
		return nil, errors.WithStack(errors.New("failed to execute callback, data is empty"))
	}
	args := strings.Split(query.Data, " ")
	if len(args) <= 0 {
		return nil, errors.WithStack(errors.New("failed to execute callback, args is not sufficient"))
	}
	callback, ok := h.callbacks[args[0]]
	if !ok {
		return h.getReplyText(query.Message, "Unknown callback"), nil
	}

	resp, err := callback(ctx, query, args[1:])
	if err != nil {
		return nil, err
	}
	ans, err := h.api.AnswerCallbackQuery(tgbotapi.CallbackConfig{
		CallbackQueryID: query.ID,
	})
	if err != nil {
		return nil, errors.WithStack(err)
	}
	if !ans.Ok {
		return nil, errors.WithStack(errors.New(string(ans.Result)))
	}
	return resp, nil
}

func (h *MessageHandler) JobSearchCallback(ctx context.Context) tgbotapi.MessageConfig {
	ctx, cancel := context.WithCancel(ctx)
	outChan, errChan := worker.Start(ctx)
	go func() {
		for {
			var resp tgbotapi.MessageConfig
			select {
			case out := <-outChan:
				careers, err := h.cleanUpSearch(out)
				if err != nil {
					logrus.Errorf("failed to clean up careers list: %v", err)
				}
				if len(careers) == 0 {
					logrus.Info("No new careers found")
					continue
				}
				resp = tgbotapi.NewMessage(h.chatID, fmt.Sprintf("I found some new careers for you: \n%s", strings.Join(careers, "\n")))
			case err := <-errChan:
				logrus.Errorf("error occurred during the job search: %v", err)
				resp = tgbotapi.NewMessage(h.chatID, "Error occurred during the job search.")
			case <-ctx.Done():
				return
			}
			_, err := h.api.Send(resp)
			if err != nil {
				logrus.Errorf("failed to send response: %v", err)
			}
		}
	}()
	h.jobSearchCancel = cancel

	return tgbotapi.NewMessage(h.chatID, "Job search has successfully started")
}

func (h *MessageHandler) cleanUpSearch(careers map[string]struct{}) ([]string, error) {
	if len(careers) == 0 {
		return nil, nil
	}
	stored, err := h.db.GetAllCareers()
	if err != nil {
		return nil, err
	}
	storedMap := make(map[string]struct{})
	for _, c := range stored {
		storedMap[c.URL] = struct{}{}
	}
	var cleanedCareers []string
	for u, _ := range careers {
		if _, ok := storedMap[u]; !ok {
			err := h.db.CreateCareers(u)
			if err != nil {
				return nil, err
			}
			cleanedCareers = append(cleanedCareers, u)
		}
	}

	return cleanedCareers, nil
}

func (h *MessageHandler) authorize(chatID int64) error {
	if chatID != h.chatID {
		return errors.New("unauthorized")
	}
	return nil
}

func getChat(upd tgbotapi.Update) int64 {
	var id int64
	if upd.Message != nil {
		id = upd.Message.Chat.ID
	}
	if upd.CallbackQuery != nil {
		id = upd.CallbackQuery.Message.Chat.ID
	}

	return id
}
