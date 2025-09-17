package services

import (
	"fmt"
	"strings"
)

// CommentFromOrderId формирует TON‑комментарий из UUID заказа.
// Формат: "ORD-XXXXXX" (последние 6 символов UUID, uppercase).
func CommentFromOrderId(orderId string) string {
	if orderId == "" {
		return ""
	}

	// убираем дефисы и переводим в upper-case
	clean := strings.ToUpper(strings.ReplaceAll(orderId, "-", ""))

	// берём последние 6 символов
	if len(clean) > 6 {
		clean = clean[len(clean)-6:]
	}

	return fmt.Sprintf("ORD-%s", clean)
}