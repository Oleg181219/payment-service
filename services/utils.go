package services


import "github.com/shopspring/decimal"



// checkAmountOk — true если фактическая сумма >= min с учетом допуска
func checkAmountOk(actual, min decimal.Decimal) bool {
	// Допуск: 1 нанотон (1e-9 TON)
	epsilon := decimal.NewFromFloat(0.000000001)
	return actual.Cmp(min.Sub(epsilon)) >= 0
}


func ptrToString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}