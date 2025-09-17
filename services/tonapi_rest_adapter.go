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

// ------------ helpers ------------

// адрес из tonutils-go может уметь отдавать raw ("0:<hex>")
// через метод StringRaw(). Опишем минимальный интерфейс для безопасной проверки.
type rawStringer interface {
	StringRaw() string
}

func normalizeTONAddr(id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return id
	}
	a, err := address.ParseAddr(id)
	if err != nil {
		// Если не смогли распарсить — возвращаем то, что дали, TonAPI ответит 400
		return id
	}
	// Возвращаем всегда RAW-адрес: "0:<hex>"
	return a.StringRaw()
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

// ------------ Event responses ------------

// Сырой ответ TonAPI для /v2/accounts/{addr}/events
type tonapiEventsResp struct {
	Events []struct {
		EventID   string            `json:"event_id"`
		Timestamp int64             `json:"timestamp"`
		Actions   []json.RawMessage `json:"actions"`
	} `json:"events"`
}

// В TonAPI action — "discriminated union".
type actionHeader struct {
	Type string `json:"type"`
	// возможные "плоские" поля
	Amount    string          `json:"amount"`
	Recipient json.RawMessage `json:"recipient"`
	Sender    json.RawMessage `json:"sender"`
	Payload   *struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"payload"`
}

// ⚡ Исправленный nestedTransfer
type nestedTransfer struct {
	Amount      any             `json:"amount"`       // ⚡ может быть string или number
	Value       any             `json:"value"`        // иногда TonAPI использует value
	Recipient   json.RawMessage `json:"recipient"`
	Destination json.RawMessage `json:"destination"`
	Sender      json.RawMessage `json:"sender"`
	Source      json.RawMessage `json:"source"`
	Comment     string          `json:"comment"`      // ⚡ новый: TonAPI кладёт коммент строкой
	Payload     *struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"payload"`
}

// ищем вложенный объект, похожий на TonTransfer
func findNestedTransfer(m map[string]json.RawMessage) *nestedTransfer {
	// типичные ключи для деталей перевода
	candidates := []string{"TonTransfer", "ton_transfer", "transfer", "tonTransfer"}
	var obj nestedTransfer
	for _, k := range candidates {
		if raw, ok := m[k]; ok && len(raw) > 0 {
			if json.Unmarshal(raw, &obj) == nil {
				return &obj
			}
		}
	}
	// fallback
	for k, raw := range m {
		if k == "type" || k == "amount" || k == "recipient" || k == "sender" || k == "payload" {
			continue
		}
		var tmp nestedTransfer
		if json.Unmarshal(raw, &tmp) == nil {
			if tmp.Amount != nil || tmp.Value != nil {
				return &tmp
			}
		}
	}
	return nil
}

// ------------ Public methods ------------

func (a *RestTonAPIAdapter) GetAccountEvents(ctx context.Context, accountID string, limit int) (Events, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	acc := normalizeTONAddr(accountID)
	u := fmt.Sprintf("%s/v2/accounts/%s/events?limit=%d", a.base, url.PathEscape(acc), limit)

	req, _ := http.NewRequestWithContext(ctx, "GET", u, nil)
	a.auth(req)

	resp, err := a.http.Do(req)
	if err != nil {
		return Events{}, err
	}
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

			amount := head.Amount
			recipient := parseAddr(head.Recipient)
			sender := parseAddr(head.Sender)
			var payload *EventPayload
			if head.Payload != nil {
				payload = &EventPayload{Type: head.Payload.Type, Text: head.Payload.Text}
			}

			// fallback через nestedTransfer
			if amount == "" || recipient == "" || sender == "" || payload == nil {
				var asMap map[string]json.RawMessage
				_ = json.Unmarshal(rawAct, &asMap)
				if tr := findNestedTransfer(asMap); tr != nil {
					// amount
					if amount == "" && tr.Amount != nil {
						switch v := tr.Amount.(type) {
						case string:
							amount = v
						case float64:
							amount = fmt.Sprintf("%.0f", v)
						case int:
							amount = fmt.Sprintf("%d", v)
						}
					}
					if amount == "" && tr.Value != nil {
						switch v := tr.Value.(type) {
						case string:
							amount = v
						case float64:
							amount = fmt.Sprintf("%.0f", v)
						case int:
							amount = fmt.Sprintf("%d", v)
						}
					}
					// recipient
					if recipient == "" {
						recipient = parseAddr(tr.Recipient)
						if recipient == "" {
							recipient = parseAddr(tr.Destination)
						}
					}
					// sender
					if sender == "" {
						sender = parseAddr(tr.Sender)
						if sender == "" {
							sender = parseAddr(tr.Source)
						}
					}
					// payload / comment
					if payload == nil {
						if tr.Payload != nil {
							payload = &EventPayload{Type: tr.Payload.Type, Text: tr.Payload.Text}
						} else if tr.Comment != "" {
							payload = &EventPayload{Type: "comment", Text: tr.Comment}
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

// ----------- Account info -------------

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