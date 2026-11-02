package dataformat

import (
	"encoding/binary"
	"fmt"
	"math"
	"strconv"
	"strings"
)

type FormatType int

const (
	FormatSignedInt FormatType = iota + 1
	FormatUnsignedInt
	FormatHexString
	FormatBinaryString
	FormatFloat32
	FormatFloat64
	FormatExpr16
	FormatExpr32
)

type wordOrder interface {
	binary.ByteOrder
	order4() [4]int
	order8() [8]int
	name() string
}

type customOrder struct {
	n4 [4]int
	n8 [8]int
	id string
}

func (o customOrder) Uint16(b []byte) uint16 {
	return binary.BigEndian.Uint16(b)
}

func (o customOrder) PutUint16(b []byte, v uint16) {
	binary.BigEndian.PutUint16(b, v)
}

func (o customOrder) Uint32(b []byte) uint32 {
	var tmp [4]byte
	for i, idx := range o.n4 {
		tmp[i] = b[idx]
	}
	return binary.BigEndian.Uint32(tmp[:])
}

func (o customOrder) PutUint32(b []byte, v uint32) {
	var tmp [4]byte
	binary.BigEndian.PutUint32(tmp[:], v)
	for i, idx := range o.n4 {
		b[idx] = tmp[i]
	}
}

func (o customOrder) order4() [4]int {
	return o.n4
}

func (o customOrder) order8() [8]int {
	return o.n8
}

func (o customOrder) Uint64(b []byte) uint64 {
	var tmp [8]byte
	for i, idx := range o.n8 {
		tmp[i] = b[idx]
	}
	return binary.BigEndian.Uint64(tmp[:])
}

func (o customOrder) PutUint64(b []byte, v uint64) {
	var tmp [8]byte
	binary.BigEndian.PutUint64(tmp[:], v)
	for i, idx := range o.n8 {
		b[idx] = tmp[i]
	}
}

func (o customOrder) name() string {
	return o.id
}

func (o customOrder) String() string {
	return o.id
}

var (
	OrderABCD binary.ByteOrder = customOrder{
		n4: [4]int{0, 1, 2, 3},
		n8: [8]int{0, 1, 2, 3, 4, 5, 6, 7},
		id: "ABCD",
	}
	OrderCDAB binary.ByteOrder = customOrder{
		n4: [4]int{2, 3, 0, 1},
		n8: [8]int{2, 3, 0, 1, 6, 7, 4, 5},
		id: "CDAB",
	}
	OrderBADC binary.ByteOrder = customOrder{
		n4: [4]int{1, 0, 3, 2},
		n8: [8]int{1, 0, 3, 2, 5, 4, 7, 6},
		id: "BADC",
	}
	OrderDCBA binary.ByteOrder = customOrder{
		n4: [4]int{3, 2, 1, 0},
		n8: [8]int{6, 7, 4, 5, 2, 3, 0, 1},
		id: "DCBA",
	}
)

type Error struct {
	Op        string
	Format    FormatType
	ByteOrder string
	Raw       []byte
	Inner     error
}

func (e *Error) Error() string {
	return fmt.Sprintf("点位数据格式化失败[%s]: 格式=%d, 字节序=%s, 原始字节=%x, 原因=%v", e.Op, e.Format, e.ByteOrder, e.Raw, e.Inner)
}

func (e *Error) Unwrap() error {
	return e.Inner
}

func FormatPoint(raw []byte, order binary.ByteOrder, fmtType FormatType, exprStr string) (string, error) {
	if len(raw) == 0 {
		return "", &Error{
			Op:        "decode",
			Format:    fmtType,
			ByteOrder: byteOrderName(order),
			Raw:       raw,
			Inner:     fmt.Errorf("原始字节为空"),
		}
	}

	switch fmtType {
	case FormatSignedInt:
		val, err := parseInteger(raw, order, true)
		if err != nil {
			return "", wrapError("decode", fmtType, order, raw, err)
		}
		return strconv.FormatInt(val, 10), nil
	case FormatUnsignedInt:
		val, err := parseUnsigned(raw, order)
		if err != nil {
			return "", wrapError("decode", fmtType, order, raw, err)
		}
		return strconv.FormatUint(val, 10), nil
	case FormatHexString:
		val, err := parseUnsigned(raw, order)
		if err != nil {
			return "", wrapError("decode", fmtType, order, raw, err)
		}
		width := len(raw) * 2
		return fmt.Sprintf("0x%0*X", width, val), nil
	case FormatBinaryString:
		val, err := parseUnsigned(raw, order)
		if err != nil {
			return "", wrapError("decode", fmtType, order, raw, err)
		}
		width := len(raw) * 8
		s := strconv.FormatUint(val, 2)
		if len(s) < width {
			s = strings.Repeat("0", width-len(s)) + s
		}
		return "0b" + s, nil
	case FormatFloat32:
		if len(raw) != 4 {
			return "", wrapError("decode", fmtType, order, raw, fmt.Errorf("Float32 需要 4 字节，实际为 %d", len(raw)))
		}
		var bits uint32
		if wo, ok := order.(wordOrder); ok {
			var buf [4]byte
			for i, idx := range wo.order4() {
				buf[i] = raw[idx]
			}
			bits = binary.BigEndian.Uint32(buf[:])
		} else {
			switch order {
			case binary.BigEndian:
				bits = binary.BigEndian.Uint32(raw)
			case binary.LittleEndian:
				bits = binary.LittleEndian.Uint32(raw)
			default:
				bits = binary.BigEndian.Uint32(raw)
			}
		}
		f := math.Float32frombits(bits)
		if math.IsInf(float64(f), 0) || math.IsNaN(float64(f)) {
			return "", wrapError("decode", fmtType, order, raw, fmt.Errorf("浮点数无效"))
		}
		return strconv.FormatFloat(float64(f), 'f', -1, 32), nil
	case FormatFloat64:
		if len(raw) != 8 {
			return "", wrapError("decode", fmtType, order, raw, fmt.Errorf("Float64 需要 8 字节，实际为 %d", len(raw)))
		}
		var bits uint64
		if wo, ok := order.(wordOrder); ok {
			var buf [8]byte
			for i, idx := range wo.order8() {
				buf[i] = raw[idx]
			}
			bits = binary.BigEndian.Uint64(buf[:])
		} else {
			switch order {
			case binary.BigEndian:
				bits = binary.BigEndian.Uint64(raw)
			case binary.LittleEndian:
				bits = binary.LittleEndian.Uint64(raw)
			default:
				bits = binary.BigEndian.Uint64(raw)
			}
		}
		f := math.Float64frombits(bits)
		if math.IsInf(f, 0) || math.IsNaN(f) {
			return "", wrapError("decode", fmtType, order, raw, fmt.Errorf("浮点数无效"))
		}
		return strconv.FormatFloat(f, 'f', -1, 64), nil
	case FormatExpr16:
		if len(raw) != 2 {
			return "", wrapError("decode", fmtType, order, raw, fmt.Errorf("表达式 16 位需要 2 字节，实际为 %d", len(raw)))
		}
		if exprStr == "" {
			return "", wrapError("decode", fmtType, order, raw, fmt.Errorf("表达式字符串为空"))
		}
		val, err := parseInteger(raw, order, true)
		if err != nil {
			return "", wrapError("decode", fmtType, order, raw, err)
		}
		res, err := evalExpression(exprStr, val)
		if err != nil {
			return "", wrapError("eval", fmtType, order, raw, err)
		}
		if res < math.MinInt16 || res > math.MaxInt16 {
			return "", wrapError("eval", fmtType, order, raw, fmt.Errorf("表达式结果超出 16 位有符号整型范围"))
		}
		return strconv.FormatInt(res, 10), nil
	case FormatExpr32:
		if len(raw) != 4 {
			return "", wrapError("decode", fmtType, order, raw, fmt.Errorf("表达式 32 位需要 4 字节，实际为 %d", len(raw)))
		}
		if exprStr == "" {
			return "", wrapError("decode", fmtType, order, raw, fmt.Errorf("表达式字符串为空"))
		}
		val, err := parseInteger(raw, order, true)
		if err != nil {
			return "", wrapError("decode", fmtType, order, raw, err)
		}
		res, err := evalExpression(exprStr, val)
		if err != nil {
			return "", wrapError("eval", fmtType, order, raw, err)
		}
		if res < math.MinInt32 || res > math.MaxInt32 {
			return "", wrapError("eval", fmtType, order, raw, fmt.Errorf("表达式结果超出 32 位有符号整型范围"))
		}
		return strconv.FormatInt(res, 10), nil
	default:
		return "", &Error{
			Op:        "decode",
			Format:    fmtType,
			ByteOrder: byteOrderName(order),
			Raw:       raw,
			Inner:     fmt.Errorf("不支持的格式类型"),
		}
	}
}

func parseInteger(raw []byte, order binary.ByteOrder, signed bool) (int64, error) {
	switch len(raw) {
	case 1:
		if signed {
			// #nosec G115 -- 单字节按补码位模式重解释为有符号数
			return int64(int8(raw[0])), nil
		}
		return int64(raw[0]), nil
	case 2:
		u := order.Uint16(raw)
		if signed {
			// #nosec G115 -- 16 位按补码位模式重解释为有符号数
			return int64(int16(u)), nil
		}
		return int64(u), nil
	case 4:
		u := order.Uint32(raw)
		if signed {
			// #nosec G115 -- 32 位按补码位模式重解释为有符号数
			return int64(int32(u)), nil
		}
		if u > math.MaxUint32 {
			return 0, fmt.Errorf("32 位整数溢出")
		}
		return int64(u), nil
	default:
		return 0, fmt.Errorf("不支持的整数字节长度: %d", len(raw))
	}
}

func parseUnsigned(raw []byte, order binary.ByteOrder) (uint64, error) {
	switch len(raw) {
	case 1:
		return uint64(raw[0]), nil
	case 2:
		return uint64(order.Uint16(raw)), nil
	case 4:
		return uint64(order.Uint32(raw)), nil
	default:
		return 0, fmt.Errorf("不支持的无符号整数字节长度: %d", len(raw))
	}
}

func wrapError(op string, fmtType FormatType, order binary.ByteOrder, raw []byte, err error) error {
	return &Error{
		Op:        op,
		Format:    fmtType,
		ByteOrder: byteOrderName(order),
		Raw:       append([]byte(nil), raw...),
		Inner:     err,
	}
}

func byteOrderName(order binary.ByteOrder) string {
	if order == nil {
		return "nil"
	}
	if wo, ok := order.(wordOrder); ok {
		return wo.name()
	}
	switch order {
	case binary.BigEndian:
		return "BigEndian"
	case binary.LittleEndian:
		return "LittleEndian"
	default:
		return "Custom"
	}
}

type tokenType int

const (
	tokenEOF tokenType = iota
	tokenNumber
	tokenVar
	tokenPlus
	tokenMinus
	tokenMul
	tokenDiv
	tokenAnd
	tokenOr
	tokenXor
	tokenShl
	tokenShr
	tokenLParen
	tokenRParen
)

type token struct {
	typ tokenType
	val int64
}

func evalExpression(expression string, v int64) (int64, error) {
	tokens, err := tokenize(expression)
	if err != nil {
		return 0, err
	}
	p := parser{tokens: tokens, v: v}
	res, err := p.parseExpression()
	if err != nil {
		return 0, err
	}
	return res, nil
}

func tokenize(s string) ([]token, error) {
	var tokens []token
	for i := 0; i < len(s); {
		ch := s[i]
		if ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r' {
			i++
			continue
		}
		switch ch {
		case '+':
			tokens = append(tokens, token{typ: tokenPlus})
			i++
		case '-':
			tokens = append(tokens, token{typ: tokenMinus})
			i++
		case '*':
			tokens = append(tokens, token{typ: tokenMul})
			i++
		case '/':
			tokens = append(tokens, token{typ: tokenDiv})
			i++
		case '&':
			tokens = append(tokens, token{typ: tokenAnd})
			i++
		case '|':
			tokens = append(tokens, token{typ: tokenOr})
			i++
		case '^':
			tokens = append(tokens, token{typ: tokenXor})
			i++
		case '<':
			if i+1 < len(s) && s[i+1] == '<' {
				tokens = append(tokens, token{typ: tokenShl})
				i += 2
			} else {
				return nil, fmt.Errorf("非法运算符: %q", s[i:])
			}
		case '>':
			if i+1 < len(s) && s[i+1] == '>' {
				tokens = append(tokens, token{typ: tokenShr})
				i += 2
			} else {
				return nil, fmt.Errorf("非法运算符: %q", s[i:])
			}
		case '(':
			tokens = append(tokens, token{typ: tokenLParen})
			i++
		case ')':
			tokens = append(tokens, token{typ: tokenRParen})
			i++
		case 'v':
			tokens = append(tokens, token{typ: tokenVar})
			i++
		default:
			if ch >= '0' && ch <= '9' {
				start := i
				for i < len(s) && s[i] >= '0' && s[i] <= '9' {
					i++
				}
				numStr := s[start:i]
				val, err := strconv.ParseInt(numStr, 10, 64)
				if err != nil {
					return nil, fmt.Errorf("数字解析失败: %w", err)
				}
				tokens = append(tokens, token{typ: tokenNumber, val: val})
			} else {
				return nil, fmt.Errorf("非法字符: %q", ch)
			}
		}
	}
	tokens = append(tokens, token{typ: tokenEOF})
	return tokens, nil
}

type parser struct {
	tokens []token
	pos    int
	v      int64
}

func (p *parser) current() token {
	if p.pos >= len(p.tokens) {
		return token{typ: tokenEOF}
	}
	return p.tokens[p.pos]
}

func (p *parser) advance() {
	if p.pos < len(p.tokens) {
		p.pos++
	}
}

func (p *parser) expect(t tokenType) error {
	if p.current().typ != t {
		return fmt.Errorf("表达式语法错误")
	}
	p.advance()
	return nil
}

func (p *parser) parseExpression() (int64, error) {
	return p.parseOr()
}

func (p *parser) parseOr() (int64, error) {
	left, err := p.parseXor()
	if err != nil {
		return 0, err
	}
	for {
		switch p.current().typ {
		case tokenOr:
			p.advance()
			right, err := p.parseXor()
			if err != nil {
				return 0, err
			}
			left = left | right
		default:
			return left, nil
		}
	}
}

func (p *parser) parseXor() (int64, error) {
	left, err := p.parseAnd()
	if err != nil {
		return 0, err
	}
	for {
		switch p.current().typ {
		case tokenXor:
			p.advance()
			right, err := p.parseAnd()
			if err != nil {
				return 0, err
			}
			left = left ^ right
		default:
			return left, nil
		}
	}
}

func (p *parser) parseAnd() (int64, error) {
	left, err := p.parseShift()
	if err != nil {
		return 0, err
	}
	for {
		switch p.current().typ {
		case tokenAnd:
			p.advance()
			right, err := p.parseShift()
			if err != nil {
				return 0, err
			}
			left = left & right
		default:
			return left, nil
		}
	}
}

func (p *parser) parseShift() (int64, error) {
	left, err := p.parseAddSub()
	if err != nil {
		return 0, err
	}
	for {
		switch p.current().typ {
		case tokenShl:
			p.advance()
			right, err := p.parseAddSub()
			if err != nil {
				return 0, err
			}
			if right < 0 || right >= 63 {
				return 0, fmt.Errorf("移位位数超出范围")
			}
			left, err = safeShl(left, uint(right))
			if err != nil {
				return 0, err
			}
		case tokenShr:
			p.advance()
			right, err := p.parseAddSub()
			if err != nil {
				return 0, err
			}
			if right < 0 || right >= 63 {
				return 0, fmt.Errorf("移位位数超出范围")
			}
			left = left >> uint(right)
		default:
			return left, nil
		}
	}
}

func (p *parser) parseAddSub() (int64, error) {
	left, err := p.parseMulDiv()
	if err != nil {
		return 0, err
	}
	for {
		switch p.current().typ {
		case tokenPlus:
			p.advance()
			right, err := p.parseMulDiv()
			if err != nil {
				return 0, err
			}
			left, err = safeAdd(left, right)
			if err != nil {
				return 0, err
			}
		case tokenMinus:
			p.advance()
			right, err := p.parseMulDiv()
			if err != nil {
				return 0, err
			}
			left, err = safeSub(left, right)
			if err != nil {
				return 0, err
			}
		default:
			return left, nil
		}
	}
}

func (p *parser) parseMulDiv() (int64, error) {
	left, err := p.parseUnary()
	if err != nil {
		return 0, err
	}
	for {
		switch p.current().typ {
		case tokenMul:
			p.advance()
			right, err := p.parseUnary()
			if err != nil {
				return 0, err
			}
			left, err = safeMul(left, right)
			if err != nil {
				return 0, err
			}
		case tokenDiv:
			p.advance()
			right, err := p.parseUnary()
			if err != nil {
				return 0, err
			}
			if right == 0 {
				return 0, fmt.Errorf("除零错误")
			}
			left, err = safeDiv(left, right)
			if err != nil {
				return 0, err
			}
		default:
			return left, nil
		}
	}
}

func (p *parser) parseUnary() (int64, error) {
	switch p.current().typ {
	case tokenPlus:
		p.advance()
		return p.parseUnary()
	case tokenMinus:
		p.advance()
		val, err := p.parseUnary()
		if err != nil {
			return 0, err
		}
		return -val, nil
	default:
		return p.parsePrimary()
	}
}

func (p *parser) parsePrimary() (int64, error) {
	switch p.current().typ {
	case tokenNumber:
		val := p.current().val
		p.advance()
		return val, nil
	case tokenVar:
		p.advance()
		return p.v, nil
	case tokenLParen:
		p.advance()
		val, err := p.parseExpression()
		if err != nil {
			return 0, err
		}
		if err := p.expect(tokenRParen); err != nil {
			return 0, err
		}
		return val, nil
	default:
		return 0, fmt.Errorf("表达式语法错误")
	}
}

func safeAdd(a, b int64) (int64, error) {
	if (b > 0 && a > math.MaxInt64-b) || (b < 0 && a < math.MinInt64-b) {
		return 0, fmt.Errorf("表达式结果溢出")
	}
	return a + b, nil
}

func safeSub(a, b int64) (int64, error) {
	if (b < 0 && a > math.MaxInt64+b) || (b > 0 && a < math.MinInt64+b) {
		return 0, fmt.Errorf("表达式结果溢出")
	}
	return a - b, nil
}

func safeMul(a, b int64) (int64, error) {
	if a == 0 || b == 0 {
		return 0, nil
	}
	if a == math.MinInt64 || b == math.MinInt64 {
		return 0, fmt.Errorf("表达式结果溢出")
	}
	absA := a
	if absA < 0 {
		absA = -absA
	}
	absB := b
	if absB < 0 {
		absB = -absB
	}
	if absA > math.MaxInt64/absB {
		return 0, fmt.Errorf("表达式结果溢出")
	}
	return a * b, nil
}

func safeDiv(a, b int64) (int64, error) {
	if b == 0 {
		return 0, fmt.Errorf("除零错误")
	}
	if a == math.MinInt64 && b == -1 {
		return 0, fmt.Errorf("表达式结果溢出")
	}
	return a / b, nil
}

func safeShl(a int64, n uint) (int64, error) {
	if n >= 63 {
		return 0, fmt.Errorf("移位位数超出范围")
	}
	if a > 0 && a > (math.MaxInt64>>n) {
		return 0, fmt.Errorf("表达式结果溢出")
	}
	if a < 0 && a < (math.MinInt64>>n) {
		return 0, fmt.Errorf("表达式结果溢出")
	}
	return a << n, nil
}
