package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/xssnick/tonutils-go/address"


)

// адрес из tonutils-go может уметь отдавать raw ("0:<hex>")
// через метод ToRaw(). Опишем минимальный интерфейс для безопасной проверки.
type rawStringer interface {
	ToRaw() string
}


func normalizeTONAddr(id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return id
	}
	a, err := address.ParseAddr(id)
	if err != nil {
		return id // TonAPI вернёт 400, а мы увидим полную ошибку в логах
	}
	// Если у типа есть ToRaw() — отдаём raw "0:<hex>" (надежно для TonAPI)
	if r, ok := any(a).(rawStringer); ok {
		return r.ToRaw()
	}
	// Фоллбек: friendly строка (может остаться UQ/EQ как в исходном)
	return a.String()
}

// маленький хелпер для тела ошибки:
func readBody(resp *http.Response) string {
	b, _ := io.ReadAll(resp.Body)
	resp.Body = io.NopCloser(bytes.NewReader(b))
	return string(b)
}



// ---------------- REST TonAPI adapter ----------------

type RestTonAPIAdapter struct {
	base  string
	token string
	http  *http.Client
}

func NewRestTonAPIAdapter(baseURL, token string) *RestTonAPIAdapter {
	return &RestTonAPIAdapter{
		base:  strings.TrimRight(baseURL, "/"),
		token: token,
		http:  &http.Client{Timeout: 10 * time.Second},
	}
}

func (a *RestTonAPIAdapter) auth(req *http.Request) {
	if a.token != "" {
		req.Header.Set("Authorization", "Bearer "+a.token)
	}
}

// parseAddr — recipient/sender в TonAPI могут быть строкой "EQ..."
// или объектом { "address": "EQ..." }. Поддержим оба формата.
func parseAddr(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	// как строка
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	// как объект
	var obj struct {
		Address string `json:"address"`
	}
	if err := json.Unmarshal(raw, &obj); err == nil {
		return obj.Address
	}
	return ""
}

// Сырой ответ TonAPI для /v2/accounts/{addr}/events
// Мы читаем только то, что нужно для TonTransfer (+комментарий).
type tonapiEventsResp struct {
	Events []struct {
		EventID   string `json:"event_id"`
		Timestamp int64  `json:"timestamp"`
		Actions   []json.RawMessage `json:"actions"`
	} `json:"events"`
}

// В TonAPI action — "discriminated union": есть поле "type"
// и под-структура по типу (например, "TonTransfer": {...}).
// Для надёжности пробуем вытащить поля из нескольких возможных мест:
// - верхний уровень action (если провайдер кладёт плоско);
// - вложенный объект с деталями (ton_transfer/TonTransfer/transfer/...).
type actionHeader struct {
	Type string `json:"type"`
	// возможные "плоские" поля (если повезёт)
	Amount    string          `json:"amount"`
	Recipient json.RawMessage `json:"recipient"`
	Sender    json.RawMessage `json:"sender"`
	Payload   *struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"payload"`
	// всё остальное — оставим для ручного поиска вложенных объектов
}

type nestedTransfer struct {
	// разные возможные поля под разные версии схемы
	Amount      string          `json:"amount"`
	Value       string          `json:"value"`
	Recipient   json.RawMessage `json:"recipient"`
	Destination json.RawMessage `json:"destination"`
	Sender      json.RawMessage `json:"sender"`
	Source      json.RawMessage `json:"source"`
	Comment     *struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"comment"`
	Payload *struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"payload"`
}

// ищем первый вложенный объект, похожий на TonTransfer
func findNestedTransfer(m map[string]json.RawMessage) *nestedTransfer {
	// типичные ключи, под которыми лежат детали тон-перевода
	candidates := []string{
		"TonTransfer", "ton_transfer", "transfer", "tonTransfer",
	}
	var obj nestedTransfer
	for _, k := range candidates {
		if raw, ok := m[k]; ok && len(raw) > 0 {
			if json.Unmarshal(raw, &obj) == nil {
				return &obj
			}
		}
	}
	// если не нашли — попробуем перебрать все вложенные объекты и взять первый,
	// в котором есть amount/value (на всякий случай)
	for k, raw := range m {
		if k == "type" || k == "amount" || k == "recipient" || k == "sender" || k == "payload" {
			continue
		}
		var tmp nestedTransfer
		if json.Unmarshal(raw, &tmp) == nil {
			if tmp.Amount != "" || tmp.Value != "" {
				return &tmp
			}
		}
	}
	return nil
}

// GetAccountEvents — подтягиваем и нормализуем события из TonAPI под наш Events.
func (a *RestTonAPIAdapter) GetAccountEvents(ctx context.Context, accountID string, limit int) (Events, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	acc := normalizeTONAddr(accountID)
	u := fmt.Sprintf("%s/v2/accounts/%s/events?limit=%d", a.base, url.PathEscape(acc), limit)

	req, _ := http.NewRequestWithContext(ctx, "GET", u, nil)
	a.auth(req)

	resp, err := a.http.Do(req)
	if err != nil { return Events{}, err }
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return Events{}, fmt.Errorf("tonapi events status %d url=%s body=%s", resp.StatusCode, u, readBody(resp))
	}

	var er tonapiEventsResp
	if err := json.NewDecoder(resp.Body).Decode(&er); err != nil {
		return Events{}, err
	}

	out := Events{Events: make([]Event, 0, len(er.Events))}
	for _, ev := range er.Events {
		ts := ev.Timestamp
		dst := Event{EventID: ev.EventID, Timestamp: &ts}

		for _, rawAct := range ev.Actions {
			// читаем заголовок action
			var head actionHeader
			_ = json.Unmarshal(rawAct, &head)
			actType := head.Type

			// по умолчанию попробуем взять "плоские" поля
			amount := head.Amount
			recipient := parseAddr(head.Recipient)
			sender := parseAddr(head.Sender)
			var payload *EventPayload
			if head.Payload != nil {
				payload = &EventPayload{Type: head.Payload.Type, Text: head.Payload.Text}
			}

			// если плоских полей нет — попробуем найти вложенный объект TonTransfer
			if amount == "" || recipient == "" || sender == "" {
				var asMap map[string]json.RawMessage
				_ = json.Unmarshal(rawAct, &asMap)
				if tr := findNestedTransfer(asMap); tr != nil {
					if amount == "" {
						if tr.Amount != "" {
							amount = tr.Amount
						} else if tr.Value != "" {
							amount = tr.Value
						}
					}
					if recipient == "" {
						recipient = parseAddr(tr.Recipient)
						if recipient == "" {
							recipient = parseAddr(tr.Destination)
						}
					}
					if sender == "" {
						sender = parseAddr(tr.Sender)
						if sender == "" {
							sender = parseAddr(tr.Source)
						}
					}
					if payload == nil {
						if tr.Payload != nil {
							payload = &EventPayload{Type: tr.Payload.Type, Text: tr.Payload.Text}
						} else if tr.Comment != nil {
							payload = &EventPayload{Type: tr.Comment.Type, Text: tr.Comment.Text}
						}
					}
				}
			}

			dst.Actions = append(dst.Actions, EventAction{
				Type:      actType,
				Amount:    amount,
				Recipient: recipient,
				Sender:    sender,
				Payload:   payload,
			})
		}
		out.Events = append(out.Events, dst)
	}
	return out, nil
}

type tonapiAccountResp struct {
	Balance int64  `json:"balance"`
	Status  string `json:"status"`
}

func (a *RestTonAPIAdapter) GetAccount(ctx context.Context, accountID string) (int64, string, error) {
	acc := normalizeTONAddr(accountID)
	u := fmt.Sprintf("%s/v2/accounts/%s", a.base, url.PathEscape(acc))
	req, _ := http.NewRequestWithContext(ctx, "GET", u, nil)
	a.auth(req)

	resp, err := a.http.Do(req)
	if err != nil {
		return 0, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return 0, "", fmt.Errorf("tonapi account status %d url=%s body=%s", resp.StatusCode, u, readBody(resp))
	}
	var ar tonapiAccountResp
	if err := json.NewDecoder(resp.Body).Decode(&ar); err != nil {
		return 0, "", err
	}
	return ar.Balance, ar.Status, nil
}

func (a *RestTonAPIAdapter) GetAccountJettonsBalances(context.Context, string) (any, error) {
	return nil, nil
}
func (a *RestTonAPIAdapter) GetAccountNftItems(context.Context, string) ([]map[string]any, error) {
	return nil, nil
}