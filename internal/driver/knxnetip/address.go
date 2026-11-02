package knxnetip

import (
	"fmt"
	"strconv"
	"strings"
)

// ParsedAddress holds a KNX group address and optional individual address / bit width.
type ParsedAddress struct {
	GroupAddr      uint16
	IndividualAddr uint16
	BitWidth       int // 0 means full byte(s); 1-7 for sub-byte DPT types
}

// ParseAddress parses group address formats:
//   - "main/middle/sub" or "main/sub" (2-level)
//   - "main/middle/sub,area.line.device" with optional ",BIT"
func ParseAddress(addr string) (*ParsedAddress, error) {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return nil, fmt.Errorf("empty address")
	}

	parts := strings.Split(addr, ",")
	groupPart := strings.TrimSpace(parts[0])
	out := &ParsedAddress{}

	group, err := parseGroupAddress(groupPart)
	if err != nil {
		return nil, err
	}
	out.GroupAddr = group

	if len(parts) > 1 {
		indPart := strings.TrimSpace(parts[1])
		if strings.Contains(indPart, ".") {
			ind, err := parseIndividualAddress(indPart)
			if err != nil {
				return nil, err
			}
			out.IndividualAddr = ind
		} else if n, err := strconv.Atoi(indPart); err == nil {
			out.BitWidth = n
		}
	}

	if len(parts) > 2 {
		if n, err := strconv.Atoi(strings.TrimSpace(parts[2])); err == nil {
			out.BitWidth = n
		}
	}

	if out.BitWidth < 0 || out.BitWidth > 7 {
		return nil, fmt.Errorf("invalid bit width: %d", out.BitWidth)
	}

	return out, nil
}

func parseGroupAddress(s string) (uint16, error) {
	s = strings.ReplaceAll(s, ".", "/")
	fields := strings.Split(s, "/")
	for i, f := range fields {
		f = strings.TrimSpace(f)
		if f == "" {
			return 0, fmt.Errorf("invalid group address: %s", s)
		}
		fields[i] = f
	}

	switch len(fields) {
	case 2:
		main, err := strconv.Atoi(fields[0])
		if err != nil {
			return 0, fmt.Errorf("invalid main group: %s", fields[0])
		}
		sub, err := strconv.Atoi(fields[1])
		if err != nil {
			return 0, fmt.Errorf("invalid sub group: %s", fields[1])
		}
		if main < 0 || main > 31 || sub < 0 || sub > 2047 {
			return 0, fmt.Errorf("group address out of range: %s", s)
		}
		return uint16(((main&0x1F)<<11 | (sub & 0x7FF)) & 0xFFFF), nil
	case 3:
		main, err := strconv.Atoi(fields[0])
		if err != nil {
			return 0, fmt.Errorf("invalid main group: %s", fields[0])
		}
		middle, err := strconv.Atoi(fields[1])
		if err != nil {
			return 0, fmt.Errorf("invalid middle group: %s", fields[1])
		}
		sub, err := strconv.Atoi(fields[2])
		if err != nil {
			return 0, fmt.Errorf("invalid sub group: %s", fields[2])
		}
		if main < 0 || main > 31 || middle < 0 || middle > 7 || sub < 0 || sub > 255 {
			return 0, fmt.Errorf("group address out of range: %s", s)
		}
		return uint16(((main&0x1F)<<11 | (middle&0x07)<<8 | (sub & 0xFF)) & 0xFFFF), nil
	default:
		return 0, fmt.Errorf("invalid group address format: %s", s)
	}
}

func parseIndividualAddress(s string) (uint16, error) {
	fields := strings.Split(s, ".")
	if len(fields) != 3 {
		return 0, fmt.Errorf("invalid individual address: %s", s)
	}
	area, err := strconv.Atoi(strings.TrimSpace(fields[0]))
	if err != nil || area < 0 || area > 15 {
		return 0, fmt.Errorf("invalid area in individual address: %s", s)
	}
	line, err := strconv.Atoi(strings.TrimSpace(fields[1]))
	if err != nil || line < 0 || line > 15 {
		return 0, fmt.Errorf("invalid line in individual address: %s", s)
	}
	device, err := strconv.Atoi(strings.TrimSpace(fields[2]))
	if err != nil || device < 0 || device > 255 {
		return 0, fmt.Errorf("invalid device in individual address: %s", s)
	}
	return uint16(((area&0x0F)<<12 | (line&0x0F)<<8 | (device & 0xFF)) & 0xFFFF), nil
}

func formatGroupAddress(addr uint16) string {
	main := (addr >> 11) & 0x1F
	middle := (addr >> 8) & 0x07
	sub := addr & 0xFF
	if middle == 0 && sub <= 255 {
		// could be 2-level or 3-level with middle=0
		if (addr&0x7FF) == sub && sub <= 255 {
			return fmt.Sprintf("%d/%d/%d", main, middle, sub)
		}
	}
	return fmt.Sprintf("%d/%d/%d", main, middle, sub)
}
