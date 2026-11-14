package pkg

import (
	"fmt"
)

// 前景 背景 颜色
// ---------------------------------------
// 30  40  黑色
// 31  41  红色
// 32  42  绿色
// 33  43  黄色
// 34  44  蓝色
// 35  45  紫红色
// 36  46  青蓝色
// 37  47  白色
//
// 代码 意义
// -------------------------
//  0  终端默认设置
//  1  高亮显示
//  4  使用下划线
//  5  闪烁
//  7  反白显示
//  8  不可见

// Text colors are ANSI foreground color codes.
const (
	TextBlack = iota + 30
	TextRed
	TextGreen
	TextYellow
	TextBlue
	TextMagenta
	TextCyan
	TextWhite
)

// Black wraps text in the ANSI black foreground color.
func Black(msg string) string {
	return SetColor(msg, 0, 0, TextBlack)
}

// Red wraps text in the ANSI red foreground color.
func Red(msg string) string {
	return SetColor(msg, 0, 0, TextRed)
}

// Green wraps text in the ANSI green foreground color.
func Green(msg string) string {
	return SetColor(msg, 0, 0, TextGreen)
}

// Yellow wraps text in the ANSI yellow foreground color.
func Yellow(msg string) string {
	return SetColor(msg, 0, 0, TextYellow)
}

// Blue wraps text in the ANSI blue foreground color.
func Blue(msg string) string {
	return SetColor(msg, 0, 0, TextBlue)
}

// Magenta wraps text in the ANSI magenta foreground color.
func Magenta(msg string) string {
	return SetColor(msg, 0, 0, TextMagenta)
}

// Cyan wraps text in the ANSI cyan foreground color.
func Cyan(msg string) string {
	return SetColor(msg, 0, 0, TextCyan)
}

// White wraps text in the ANSI white foreground color.
func White(msg string) string {
	return SetColor(msg, 0, 0, TextWhite)
}

// SetColor wraps text in ANSI color and reset sequences.
func SetColor(msg string, conf, bg, text int) string {
	return fmt.Sprintf("%c[%d;%d;%dm%s%c[0m", 0x1B, conf, bg, text, msg, 0x1B)
}
