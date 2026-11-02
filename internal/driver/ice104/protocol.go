package ice104

// IEC 60870-5-104 APCI / ASDU constants used by the driver.

const (
	startByte = 0x68

	uStartDTAct byte = 0x07
	uStartDTCon byte = 0x0B
	uStopDTAct  byte = 0x13
	uStopDTCon  byte = 0x23
	uTestFRAct  byte = 0x43
	uTestFRCon  byte = 0x83

	cotActivation    uint16 = 6
	cotActivationCon uint16 = 7
	cotInterrogated  uint16 = 20

	typeM_SP_NA_1 = 1
	typeM_ME_NA_1 = 9
	typeM_ME_NB_1 = 11
	typeM_ME_NC_1 = 13
	typeM_IT_NA_1 = 15
	typeC_IC_NA_1 = 100
	typeC_SC_NA_1 = 45
)

var typeIDByName = map[string]byte{
	"M_SP_NA_1": typeM_SP_NA_1,
	"M_ME_NA_1": typeM_ME_NA_1,
	"M_ME_NB_1": typeM_ME_NB_1,
	"M_ME_NC_1": typeM_ME_NC_1,
	"M_IT_NA_1": typeM_IT_NA_1,
	"C_SC_NA_1": typeC_SC_NA_1,
}

func resolveTypeID(group, dataType string) byte {
	if id, ok := typeIDByName[group]; ok {
		return id
	}
	switch dataType {
	case "BOOL", "BIT":
		return typeM_SP_NA_1
	case "INT16", "UINT16":
		return typeM_ME_NB_1
	case "INT32", "UINT32":
		return typeM_IT_NA_1
	default:
		return typeM_ME_NC_1
	}
}

func encodeIOA(ioa uint32) []byte {
	return []byte{byte(ioa & 0xFF), byte((ioa >> 8) & 0xFF), byte((ioa >> 16) & 0xFF)}
}

func decodeIOA(b []byte) uint32 {
	if len(b) < 3 {
		return 0
	}
	return uint32(b[0]) | uint32(b[1])<<8 | uint32(b[2])<<16
}
