export const baseWordOrderOptions = [
    { label: 'AB (Big-Endian)', value: 'AB' },
    { label: 'BA (Little-Endian)', value: 'BA' },
    { label: 'ABCD (Big-Endian 双字)', value: 'ABCD' },
    { label: 'BADC (Mid-Big-Endian)', value: 'BADC' },
    { label: 'CDAB (Mid-Little-Endian)', value: 'CDAB' },
    { label: 'DCBA (Little-Endian 双字)', value: 'DCBA' }
]

export const baseParseTypeOptions = [
    { label: 'BIT', value: 'BIT', bytes: 1 },
    { label: 'UINT8', value: 'UINT8', bytes: 1 },
    { label: 'INT8', value: 'INT8', bytes: 1 },
    { label: 'BCD8', value: 'BCD8', bytes: 1 },
    { label: 'UINT16', value: 'UINT16', bytes: 2 },
    { label: 'INT16', value: 'INT16', bytes: 2 },
    { label: 'UINT16_SWAP', value: 'UINT16_SWAP', bytes: 2 },
    { label: 'INT16_SWAP', value: 'INT16_SWAP', bytes: 2 },
    { label: 'BCD16', value: 'BCD16', bytes: 2 },
    { label: 'FLOAT16', value: 'FLOAT16', bytes: 2 },
    { label: 'UINT32', value: 'UINT32', bytes: 4 },
    { label: 'INT32', value: 'INT32', bytes: 4 },
    { label: 'UINT32_SWAP', value: 'UINT32_SWAP', bytes: 4 },
    { label: 'INT32_SWAP', value: 'INT32_SWAP', bytes: 4 },
    { label: 'FLOAT32', value: 'FLOAT32', bytes: 4 },
    { label: 'FLOAT32_SWAP', value: 'FLOAT32_SWAP', bytes: 4 },
    { label: 'BCD32', value: 'BCD32', bytes: 4 },
    { label: 'UINT64', value: 'UINT64', bytes: 8 },
    { label: 'INT64', value: 'INT64', bytes: 8 },
    { label: 'FLOAT64', value: 'FLOAT64', bytes: 8 },
    { label: 'FLOAT64_SWAP', value: 'FLOAT64_SWAP', bytes: 8 },
    { label: 'STRING', value: 'STRING', bytes: 0 }
]

export const getWordOrderOptionsForBytes = (byteLength) => {
    if (byteLength === 1) {
        return []
    }
    if (byteLength === 2) {
        return baseWordOrderOptions.filter(o => o.value === 'AB' || o.value === 'BA')
    }
    if (byteLength === 4) {
        return baseWordOrderOptions.filter(o => o.value === 'ABCD' || o.value === 'BADC' || o.value === 'CDAB' || o.value === 'DCBA')
    }
    return baseWordOrderOptions.filter(o => o.value === 'ABCD' || o.value === 'BADC' || o.value === 'CDAB' || o.value === 'DCBA')
}

export const filterParseTypesByBytes = (byteLength) => {
    if (!byteLength) {
        return baseParseTypeOptions
    }
    return baseParseTypeOptions.filter(o => o.bytes === byteLength)
}

export const wordOrderToBackend = (value) => {
    if (value === 'AB' || value === 'BA') {
        if (value === 'AB') return 'ABCD'
        return 'DCBA'
    }
    return value || ''
}

export const reorderBytes = (bytes, byteLength, wordOrder) => {
    if (!bytes || bytes.length === 0) return []
    if (!byteLength || byteLength === 1) return bytes
    if (byteLength === 2) {
        if (wordOrder === 'BA') {
            return [bytes[1], bytes[0]]
        }
        return bytes
    }
    if (bytes.length < 4) return bytes
    const w1 = bytes.slice(0, 2)
    const w2 = bytes.slice(2, 4)
    const w3 = bytes.slice(4, 6)
    const w4 = bytes.slice(6, 8)
    switch (wordOrder) {
        case 'BADC':
            return [...w2, ...w1, ...w4, ...w3].slice(0, byteLength)
        case 'CDAB':
            return [...w3, ...w4, ...w1, ...w2].slice(0, byteLength)
        case 'DCBA':
            return [...w4, ...w3, ...w2, ...w1].slice(0, byteLength)
        default:
            return bytes
    }
}

export const bcdToInt = (value) => {
    let v = typeof value === 'number' ? value : 0
    let result = 0
    let multiplier = 1
    while (v > 0) {
        const digit = v & 0xF
        if (digit > 9) return 0
        result += digit * multiplier
        multiplier *= 10
        v >>= 4
    }
    return result
}

export const parseByType = (bytes, type) => {
    if (!bytes) return undefined
    if (type === 'STRING') {
        return String.fromCharCode(...bytes)
    }
    const view = new DataView(new Uint8Array(bytes).buffer)
    switch (type) {
        case 'BIT':
            return bytes[0] & 0x01
        case 'UINT8':
            return bytes[0]
        case 'INT8':
            return (bytes[0] << 24) >> 24
        case 'UINT16':
        case 'UINT16_SWAP':
            return view.getUint16(0, type.endsWith('SWAP'))
        case 'INT16':
        case 'INT16_SWAP':
            return view.getInt16(0, type.endsWith('SWAP'))
        case 'UINT32':
        case 'UINT32_SWAP':
            return view.getUint32(0, type.endsWith('SWAP'))
        case 'INT32':
        case 'INT32_SWAP':
            return view.getInt32(0, type.endsWith('SWAP'))
        case 'FLOAT32':
        case 'FLOAT32_SWAP':
            return view.getFloat32(0, type.endsWith('SWAP'))
        case 'UINT64':
        case 'INT64':
        case 'FLOAT64':
        case 'FLOAT64_SWAP':
            return undefined
        case 'BCD8':
            return bcdToInt(bytes[0])
        case 'BCD16':
            return bcdToInt(view.getUint16(0, false))
        case 'BCD32':
            return bcdToInt(view.getUint32(0, false))
        case 'FLOAT16':
            return undefined
        default:
            return undefined
    }
}

export const applyFormula = (raw, expr, scale, offset) => {
    let v = raw
    if (v === undefined || v === null || isNaN(Number(v))) {
        return raw
    }
    let result = Number(v)
    const s = typeof scale === 'number' ? scale : 1
    const o = typeof offset === 'number' ? offset : 0
    result = result * s + o
    const formula = (expr || '').trim()
    if (!formula) {
        return result
    }
    try {
         
        const fn = new Function('v', `return ${formula}`)
        return fn(result)
    } catch (e) {
        return result
    }
}

export const registersToBytes = (registers) => {
    if (!Array.isArray(registers) || registers.length === 0) {
        return []
    }
    const bytes = []
    for (const r of registers) {
        let v = Number(r)
        if (!Number.isFinite(v)) {
            continue
        }
        v = v & 0xFFFF
        bytes.push((v >> 8) & 0xFF)
        bytes.push(v & 0xFF)
    }
    return bytes
}
