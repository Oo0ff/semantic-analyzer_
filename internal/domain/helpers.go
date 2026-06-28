package domain

// FixedPointScale — глобальный масштаб для fixed-point арифметики
// (все Confidence, Score, Weight и т.д. хранятся как int64 * FixedPointScale)
const FixedPointScale = 1000000

// Min возвращает меньшее из двух целых чисел
func Min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// Max возвращает большее из двух целых чисел
func Max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// Abs возвращает абсолютное значение
func Abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// ToFixedPoint конвертирует float64 [0..1] в int64 fixed-point
func ToFixedPoint(f float64) int64 {
	return int64(f * float64(FixedPointScale))
}

// FromFixedPoint конвертирует int64 fixed-point обратно в float64 [0..1]
func FromFixedPoint(i int64) float64 {
	return float64(i) / float64(FixedPointScale)
}