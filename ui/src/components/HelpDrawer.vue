<template>
  <a-drawer
    :visible="visible"
    :width="720"
    :footer="false"
    unmount-on-close
    class="help-drawer"
    render-to-body
    @update:visible="(v) => emit('update:visible', v)"
    @cancel="onCancel"
  >
    <template #title>
      <span class="help-drawer__title">帮助文档</span>
    </template>

    <article class="help-doc">
      <header class="help-doc__hero">
        <span class="protocol-tag protocol-tag--accent">{{ formatProtocolTag(channelProtocol) }}</span>
        <p class="help-doc__lead">点位配置规范与常见问题说明。</p>
      </header>

      <div class="help-doc__sections">

        <!-- 通用点位配置字段（所有南向协议共用） -->
        <section class="help-doc-section">
          <h2 class="help-doc-section__title">通用点位配置字段（所有协议共用）</h2>
          <p class="help-doc-section__text">
            无论 Modbus / BACnet / OPC-UA / S7 / EtherNet/IP 等何种南向协议，网关对"点位值"的处理模型是统一的。
            下方字段决定<strong>原始数据如何被解析、换算、以及对外暴露成什么类型</strong>，是点位配置的核心，需重点理解。
          </p>
          <div class="help-doc-card">
            <div class="help-doc-row help-doc-row--stack">
              <div class="help-doc-row__label">
                <strong>字节数 (byteLength)</strong>
                <span>1 / 2 / 4 / 8</span>
              </div>
              <div class="help-doc-row__desc">
                点位占用的字节长度，该值自动过滤下方可用的"解析类型"：
                <ul class="help-doc-optlist">
                  <li><code>1</code><span>1 字节（位 / 字节型）</span></li>
                  <li><code>2</code><span>16 位（1 寄存器）</span></li>
                  <li><code>4</code><span>32 位（2 寄存器，浮点 / 长整型）</span></li>
                  <li><code>8</code><span>64 位（4 寄存器，双精度）</span></li>
                </ul>
              </div>
            </div>
            <div class="help-doc-row help-doc-row--stack">
              <div class="help-doc-row__label">
                <strong>数据格式（格式预设）</strong>
                <span>format preset</span>
              </div>
              <div class="help-doc-row__desc">
                一组常用组合（Signed / Unsigned / FloatABCD / DoubleABCD …），选中后<strong>一次性</strong>自动填充字节数、解析类型、数据类型与字序。
                新手建议先选预设，再微调；手动逐项配置时务必保证三者自洽。
              </div>
            </div>
            <div class="help-doc-row help-doc-row--stack">
              <div class="help-doc-row__label">
                <strong>解析类型（UI 显示）</strong>
                <span>parseType</span>
              </div>
              <div class="help-doc-row__desc">
                决定"<strong>原始字节如何被解读为数值</strong>"，用于前端预览与校验。选项随字节数变化：
                <ul class="help-doc-optlist">
                  <li><code>1B</code><span>BIT / UINT8 / INT8 / BCD8</span></li>
                  <li><code>2B</code><span>UINT16 / INT16 及其 _SWAP、BCD16、FLOAT16</span></li>
                  <li><code>4B</code><span>UINT32 / INT32 / FLOAT32 及其 _SWAP、BCD32</span></li>
                  <li><code>8B</code><span>UINT64 / INT64 / FLOAT64（及 _SWAP）</span></li>
                </ul>
                <strong>_SWAP</strong> 表示在所选字序基础上再做一次字内字节交换（等价于 AB↔BA）。
              </div>
            </div>
            <div class="help-doc-row help-doc-row--stack">
              <div class="help-doc-row__label">
                <strong>数据类型（设备协议 / 存储）</strong>
                <span>datatype</span>
              </div>
              <div class="help-doc-row__desc">
                决定点位值在网关内部与<strong>北向接口</strong>（MQTT / OPC-UA / Sparkplug 等）中的存储与对外类型。<strong>必须与"解析类型"匹配</strong>：
                <ul class="help-doc-optlist">
                  <li><code>FLOAT32</code><span>→ float32</span></li>
                  <li><code>INT16</code><span>→ int16</span></li>
                  <li><code>UINT16</code><span>→ uint16</span></li>
                  <li><code>FLOAT64</code><span>→ float64</span></li>
                  <li><code>STRING</code><span>→ string</span></li>
                </ul>
                两者不一致（如解析选 FLOAT32 但 datatype 选 int32）会导致北向拿到错误类型，是最常见的配置错误。
              </div>
            </div>
            <div class="help-doc-row help-doc-row--stack">
              <div class="help-doc-row__label">
                <strong>字序 WordOrder</strong>
                <span>AB / BA / ABCD / BADC / CDAB / DCBA</span>
              </div>
              <div class="help-doc-row__desc">
                字节数 ≥ 4（多字）时决定"字"的排列顺序；字节数 = 2 时仅 AB/BA（字内字节序）：
                <ul class="help-doc-optlist">
                  <li><code>ABCD</code><span>大端双字（最常用）</span></li>
                  <li><code>DCBA</code><span>小端双字（x86 风格，完全反转）</span></li>
                  <li><code>CDAB / BADC</code><span>中间态，部分仪表 / PLC 使用</span></li>
                </ul>
                字序选错会让浮点与长整型数值完全错乱（乱码或极大值）。
              </div>
            </div>
            <div class="help-doc-row help-doc-row--stack">
              <div class="help-doc-row__label">
                <strong>缩放比例 / 偏移量</strong>
                <span>scale / offset</span>
              </div>
              <div class="help-doc-row__desc">
                最终值 = <code>原始值 × scale + offset</code>。用于工程量转换，例如温湿度变送器原始 <code>707</code> 表示 <code>70.7%RH</code>，设 <strong>scale=0.1</strong>。两者在解析之后、公式之前生效。
              </div>
            </div>
            <div class="help-doc-row help-doc-row--stack">
              <div class="help-doc-row__label">
                <strong>读/写公式（变量 v）</strong>
                <span>read_formula / write_formula</span>
              </div>
              <div class="help-doc-row__desc">
                在缩放之后对变量 <code>v</code> 做任意表达式，如 <code>v * 0.1</code>、<code>Math.round(v)</code>、<code>(v - 32) * 5 / 9</code>。读公式作用于采集方向，写公式作用于下发方向。
              </div>
            </div>
            <div class="help-doc-row help-doc-row--stack">
              <div class="help-doc-row__label">
                <strong>单位 / 默认值 / 读写权限</strong>
                <span>unit / defaultValue / readwrite</span>
              </div>
              <div class="help-doc-row__desc">
                单位仅用于显示标注（%RH、℃ 等）；默认值是采集失败时的兜底展示值；
                读写权限 <strong>R</strong>=只读（输入类点位常用），<strong>RW</strong>=可读写（保持类点位可写）。
              </div>
            </div>
          </div>
          <p class="help-doc-example">
            各协议下方还会单独说明<strong>专属的"地址"字段</strong>如何填写（如 Modbus 的寄存器偏移、S7 的 DB 地址、SNMP 的 OID 等），
            以及该协议的<strong>通道连接参数</strong>。通用字段与协议地址相互配合，才能正确采到值。
          </p>
        </section>

        <!-- Modbus -->
        <template v-if="channelProtocol.includes('modbus')">
          <section class="help-doc-section">
            <h2 class="help-doc-section__title">协议介绍</h2>
            <p class="help-doc-section__text">
              Modbus 是工业自动化最常用的通信协议，分为 <strong>Modbus-RTU</strong>（串口 RS-232/RS-485）与
              <strong>Modbus-TCP</strong>（以太网）两种传输方式。两者的点位配置逻辑完全一致，区别仅在于
              <strong>物理链路与寻址</strong>：RTU 通过串口参数（波特率/校验/停止位）与从站地址（Slave ID）通信，
              TCP 通过 IP + Unit ID 通信。本说明以 Modbus-RTU 为重点，覆盖串口点位配置的全流程。
            </p>
          </section>

          <section class="help-doc-section">
            <h2 class="help-doc-section__title">寄存器地址规范</h2>
            <div class="help-doc-card">
              <div v-for="item in modbusRegisterSpecs" :key="item.meta" class="help-doc-row">
                <div class="help-doc-row__label">
                  <strong>{{ item.name }}</strong>
                  <span>{{ item.meta }}</span>
                </div>
                <code class="help-doc-row__value">{{ item.range }}</code>
              </div>
            </div>
            <p class="help-doc-example">
              协议文档中的"寄存器号"需减去基地址得到 <strong>PDU 偏移</strong>：保持寄存器
              <code>40001 → 0</code>、<code>40002 → 1</code>；输入寄存器 <code>30001 → 0</code>；线圈
              <code>00001 → 0</code>；离散输入 <code>10001 → 0</code>。点位"地址"栏填写的即是这个偏移量。
            </p>
          </section>

          <section class="help-doc-section">
            <h2 class="help-doc-section__title">Modbus 配置要点</h2>
            <p class="help-doc-section__text">
              字节数 / 解析类型 / 数据类型 / 字序 / 缩放 / 公式 / 单位等<strong>通用字段</strong>已统一在上方
              <strong>「通用点位配置字段」</strong>一节说明，Modbus 请重点关注下面两点：
            </p>
            <div class="help-doc-card">
              <div class="help-doc-row help-doc-row--stack">
                <div class="help-doc-row__label">
                  <strong>寄存器类型 ↔ 功能码</strong>
                  <span>0x01 / 0x02 / 0x03 / 0x04</span>
                </div>
                <div class="help-doc-row__desc">
                  线圈=0x01、离散输入=0x02、保持寄存器=0x03、输入寄存器=0x04。功能码由寄存器类型决定，无需手填。
                </div>
              </div>
              <div class="help-doc-row help-doc-row--stack">
                <div class="help-doc-row__label">
                  <strong>地址 = PDU 偏移</strong>
                  <span>40001 → 0</span>
                </div>
                <div class="help-doc-row__desc">
                  点位"地址"栏填 PDU 偏移（协议号减基地址），不是 40001 这类"寄存器号"。
                </div>
              </div>
            </div>
          </section>

          <section class="help-doc-section">
            <h2 class="help-doc-section__title">解析类型 ↔ 数据类型 对照</h2>
            <div class="help-doc-card">
              <div v-for="item in modbusParseTypeMap" :key="item.parse" class="help-doc-row help-doc-row--stack">
                <div class="help-doc-row__label">
                  <strong>{{ item.parse }}</strong>
                  <span>→ datatype: {{ item.datatype }}</span>
                </div>
                <div class="help-doc-row__desc">{{ item.usage }}</div>
              </div>
            </div>
          </section>

          <section class="help-doc-section">
            <h2 class="help-doc-section__title">Modbus-RTU 串口与从站配置</h2>
            <div class="help-doc-card">
              <div class="help-doc-row help-doc-row--stack">
                <div class="help-doc-row__label">
                  <strong>串口参数</strong>
                  <span>baudrate / dataBits / stopBits / parity</span>
                </div>
                <div class="help-doc-row__desc">
                  波特率（常用 4800/9600）、数据位（通常 8）、停止位（通常 1）、校验（None/Even/Odd）。
                  <strong>必须与设备完全一致</strong>，否则串口无法打开或通信失败（表现为通道 offline、点位全 Bad）。
                </div>
              </div>
              <div class="help-doc-row help-doc-row--stack">
                <div class="help-doc-row__label">
                  <strong>从站地址 (Slave ID)</strong>
                  <span>1 – 247</span>
                </div>
                <div class="help-doc-row__desc">
                  RTU 在串口总线上通过地址寻址；TCP 则对应 Unit ID。与点位"地址"（寄存器偏移）是两个不同概念，勿混淆。
                </div>
              </div>
              <div class="help-doc-row help-doc-row--stack">
                <div class="help-doc-row__label">
                  <strong>功能码 (Function Code)</strong>
                  <span>0x01 / 0x02 / 0x03 / 0x04</span>
                </div>
                <div class="help-doc-row__desc">
                  由寄存器类型决定：线圈=0x01、离散输入=0x02、保持寄存器=0x03、输入寄存器=0x04。
                  用 pyserial 直接读设备能通、网关却读不到时，多半是串口参数/从站地址/功能码不匹配。
                </div>
              </div>
            </div>
          </section>

          <section class="help-doc-section">
            <h2 class="help-doc-section__title">点位转换与地址映射</h2>
            <p class="help-doc-section__text">
              网关按 "寄存器类型 + PDU 偏移" 组织请求。填写建议：
            </p>
            <ol class="help-doc-steps">
              <li>从协议文档确认寄存器类型（保持/输入/线圈/离散输入）与寄存器号；</li>
              <li>减去基地址得到偏移（如保持寄存器 40001 → 0）填入点位"地址"；</li>
              <li>按数据宽度设置字节数（float32 = 4、int16 = 2、double = 8）；</li>
              <li>按设备文档选择字序，并用"快速验证"对照原始报文确认数值；</li>
              <li>必要时配置 scale/offset/公式，把原始值转为工程量。</li>
            </ol>
            <p class="help-doc-example">
              例：某 485 温湿度变送器地址 1、9600/8/N/1，保持寄存器 40001（湿度，无符号）与 40002（温度，有符号），
              原始值需 ÷10。配置：字节数=2、解析类型=UINT16、数据类型=uint16、地址=0、scale=0.1（湿度）；
              温度则解析类型=INT16、数据类型=int16、地址=1、scale=0.1。
            </p>
          </section>

          <!-- 完整配置示例：485 温湿度变送器 -->
          <section class="help-doc-section">
            <h2 class="help-doc-section__title">完整配置示例：485 温湿度变送器</h2>
            <p class="help-doc-section__text">
              下面以一份真实设备手册为例，演示从<strong>协议文档</strong>逐字段映射到网关点位配置的全过程。
              设备：485 型温湿度变送器，默认地址 1、波特率 4800、8 位数据位 / 1 位停止位 / 无校验、Modbus-RTU。
            </p>

            <h3 class="help-doc-subtitle">① 设备手册要点</h3>
            <div class="help-doc-card">
              <div class="help-doc-row help-doc-row--stack">
                <div class="help-doc-row__label">
                  <strong>寄存器表</strong>
                  <span>0000 H / 0001 H</span>
                </div>
                <div class="help-doc-row__desc">
                  湿度 = 40001（实际值 ×10，16 位无符号，只读）；温度 = 40002（实际值 ×10，16 位<strong>有符号补码</strong>，只读）。
                </div>
              </div>
              <div class="help-doc-row help-doc-row--stack">
                <div class="help-doc-row__label">
                  <strong>应答帧示例</strong>
                  <span>01 03 04 0292 FF9B 5A 3D</span>
                </div>
                <div class="help-doc-row__desc">
                  湿度字节 <code>02 92</code> = 658；温度字节 <code>FF 9B</code> = 有符号 -101（低于 0℃ 时以补码上传）。
                </div>
              </div>
            </div>

            <h3 class="help-doc-subtitle">② 完整字段映射</h3>
            <p class="help-doc-example">
              <strong>通道（Modbus-RTU 链路）</strong>：协议 Modbus-RTU、Slave ID <code>1</code>、波特率 <code>4800</code>、数据位 <code>8</code>、停止位 <code>1</code>、校验 <code>None</code>。
            </p>
            <table class="help-doc-table">
              <thead>
                <tr>
                  <th>点位字段</th>
                  <th>湿度</th>
                  <th>温度</th>
                  <th>手册依据</th>
                </tr>
              </thead>
              <tbody>
                <tr><td>name</td><td>湿度 / Humidity</td><td>温度 / Temperature</td><td>内容列</td></tr>
                <tr><td>register_type</td><td>holding</td><td>holding</td><td>40001/40002 = 保持寄存器</td></tr>
                <tr><td>function_code</td><td>3</td><td>3</td><td>问询帧 0x03</td></tr>
                <tr><td>address</td><td>0</td><td>1</td><td>寄存器 0000H/0001H（零基偏移）</td></tr>
                <tr><td>byteLength</td><td>2</td><td>2</td><td>16 位 = 1 寄存器</td></tr>
                <tr><td>parseType</td><td>UINT16</td><td>INT16</td><td>无符号 / 有符号补码</td></tr>
                <tr><td>datatype</td><td>uint16</td><td>int16</td><td>落库与北向类型</td></tr>
                <tr><td>word_order</td><td>AB</td><td>AB</td><td>单字大端，应答帧高位在前</td></tr>
                <tr><td>scale</td><td>0.1</td><td>0.1</td><td>"实际值 ×10" → 实际 = 原始 ×0.1</td></tr>
                <tr><td>offset</td><td>0</td><td>0</td><td>—</td></tr>
                <tr><td>unit</td><td>%RH</td><td>℃</td><td>—</td></tr>
                <tr><td>readwrite</td><td>R</td><td>R</td><td>手册"只读"</td></tr>
              </tbody>
            </table>

            <h3 class="help-doc-subtitle">③ 本地转换与 UI 显示</h3>
            <p class="help-doc-section__text">
              <strong>解析类型（parseType）</strong>就是"设备协议原始值 → <strong>本地转换后用于 UI 显示</strong>"的字段：
              它决定原始字节如何被解读为数值，再经 <code>scale</code> 缩放后直接呈现给用户（文档中标注为"UI 显示"）。
              <code>datatype</code> 则是该值落库、对外（北向）暴露的类型，两者必须一致。
            </p>
            <div class="help-doc-card">
              <div class="help-doc-row help-doc-row--stack">
                <div class="help-doc-row__label"><strong>湿度</strong><span>本地转换链</span></div>
                <div class="help-doc-row__desc">
                  原始报文 <code>02 92</code> → <code>parseType=UINT16</code> 解读为 <code>658</code> → <code>scale=0.1</code> → <strong>UI 显示 65.8 %RH</strong>
                </div>
              </div>
              <div class="help-doc-row help-doc-row--stack">
                <div class="help-doc-row__label"><strong>温度</strong><span>本地转换链</span></div>
                <div class="help-doc-row__desc">
                  原始报文 <code>FF 9B</code> → <code>parseType=INT16</code> 按补码解读为 <code>-101</code> → <code>scale=0.1</code> → <strong>UI 显示 -10.1 ℃</strong>
                  （负数补码无需任何公式，选对 INT16 即可）
                </div>
              </div>
            </div>

            <h3 class="help-doc-subtitle">④ 应答帧反算验证</h3>
            <p class="help-doc-example">
              对照设备应答帧 <code>01 03 04 0292 FF9B …</code> 校验：湿度 <code>02 92</code>=658→×0.1=<strong>65.8 %RH</strong> ✓；
              温度 <code>FF 9B</code>=-101→×0.1=<strong>-10.1 ℃</strong> ✓。若你用网关的"快速验证"读到相同字节，配置即正确。
            </p>
          </section>

          <section class="help-doc-section">
            <h2 class="help-doc-section__title">常见问题</h2>
            <a-collapse class="help-doc-faq" :bordered="false" expand-icon-position="right">
              <a-collapse-item header="为什么数值显示为 0 或不变?" key="1">
                先确认通道已 online；再核对从站地址、功能码与寄存器偏移（Offset）是否正确。RTU 还需检查串口参数与
                485 接线（A/B 是否反接）。用 pyserial 直接读设备能通、网关却读不到，通常是串口参数/从站地址不匹配。
              </a-collapse-item>
              <a-collapse-item header="如何配置浮点数 (Float32)?" key="2">
                字节数必须设为 <strong>4</strong>（占用 2 个连续寄存器），解析类型选 <strong>FLOAT32</strong>、
                数据类型选 <strong>float32</strong>，并按设备文档设置字序（ABCD / DCBA / CDAB / BADC）。
              </a-collapse-item>
              <a-collapse-item header="浮点数显示为乱码或极大值?" key="3">
                绝大多数是<strong>字序选错</strong>或<strong>字节数误设为 2</strong>。先在"快速验证"里对照原始 4 字节报文，
                确认 ABCD 等排列方式是否与设备一致；多寄存器数值切勿用两个独立 16 位点位自行拼装。
              </a-collapse-item>
              <a-collapse-item header="解析类型与数据类型要怎么配?" key="4">
                两者需一一对应：FLOAT32→float32、INT16→int16、UINT16→uint16、FLOAT64→float64、STRING→string。
                解析类型决定"怎么读"，数据类型决定"对外是什么类型"，不匹配会导致北向订阅方类型错误。
              </a-collapse-item>
              <a-collapse-item header="Modbus-RTU 通道一直 offline?" key="5">
                串口参数（波特率/校验/停止位/数据位）必须与设备完全一致；确认 /dev/ttySx 等设备文件存在且进程有读权限。
                网关底层按 rtu:///dev/ttySx?baudrate=…&parity=… 形式打开串口，任一参数不对都会打不开。
              </a-collapse-item>
              <a-collapse-item header="32 位值被拆成高低字怎么办?" key="6">
                直接用 <strong>INT32 / UINT32 / FLOAT32</strong> + 正确的字序解析，不要拆成两个 16 位点位手动拼装，
                后者极易因字序/越界而出错。
              </a-collapse-item>
            </a-collapse>
          </section>
        </template>

        <!-- BACnet -->
        <template v-else-if="channelProtocol.includes('bacnet')">
          <section class="help-doc-section">
            <h2 class="help-doc-section__title">协议介绍</h2>
            <p class="help-doc-section__text">
              BACnet（楼宇自动化与控制网络）是智能建筑、HVAC、照明、安防领域的标准协议，基于 <strong>UDP/IP</strong> 通信（默认端口 47808）。
              网关作为 <strong>BACnet 客户端</strong>去读取现场设备（如 DDC、温控器、电表）的对象属性。
            </p>
          </section>

          <section class="help-doc-section">
            <h2 class="help-doc-section__title">对象类型与数据类型</h2>
            <div class="help-doc-card">
              <div v-for="item in bacnetObjectSpecs" :key="item.value" class="help-doc-row">
                <div class="help-doc-row__label">
                  <strong>{{ item.name }}</strong>
                </div>
                <code class="help-doc-row__value">{{ item.value }}</code>
              </div>
            </div>
            <div class="help-doc-card" style="margin-top:12px">
              <div v-for="item in bacnetDatatypeSpecs" :key="item.name" class="help-doc-row">
                <div class="help-doc-row__label">
                  <strong>{{ item.name }}</strong>
                </div>
                <code class="help-doc-row__value">{{ item.value }}</code>
              </div>
            </div>
          </section>

          <section class="help-doc-section">
            <h2 class="help-doc-section__title">点位地址格式</h2>
            <p class="help-doc-section__text">
              地址由 <strong>"对象类型:实例号"</strong> 组成，例如 <code>AnalogInput:1</code>、<code>BinaryOutput:3</code>。
              实例号是设备内对象的唯一标识（由厂商或设备配置分配）。默认读取对象属性 <code>present-value</code>。
            </p>
            <p class="help-doc-example">
              例：某温控器的"回风温度"为 AnalogInput 对象、实例号 1 → 地址填 <code>AnalogInput:1</code>，数据类型选 <code>real</code>（浮点）。
            </p>
          </section>

          <section class="help-doc-section">
            <h2 class="help-doc-section__title">通道连接配置</h2>
            <div class="help-doc-card">
              <div class="help-doc-row help-doc-row--stack">
                <div class="help-doc-row__label">
                  <strong>本机 IP（绑定）</strong>
                  <span>interface_ip</span>
                </div>
                <div class="help-doc-row__desc">
                  网关用于收发 BACnet 报文的本机网卡 IP；多网卡环境需指定，留空则自动选择。
                </div>
              </div>
              <div class="help-doc-row help-doc-row--stack">
                <div class="help-doc-row__label">
                  <strong>远程目标 IP</strong>
                  <span>target_ip</span>
                </div>
                <div class="help-doc-row__desc">
                  现场 BACnet 设备 / 路由器 IP，例如 192.168.3.115。
                </div>
              </div>
              <div class="help-doc-row help-doc-row--stack">
                <div class="help-doc-row__label">
                  <strong>端口</strong>
                  <span>port，默认 47808</span>
                </div>
                <div class="help-doc-row__desc">
                  BACnet/IP 默认 UDP 47808（0xBAC0）；跨网段经 BBMD 时需与设备侧一致。
                </div>
              </div>
              <div class="help-doc-row help-doc-row--stack">
                <div class="help-doc-row__label">
                  <strong>加密参数（可选）</strong>
                  <span>key / cert / ca</span>
                </div>
                <div class="help-doc-row__desc">
                  若设备启用 BACnet/SC 或 TLS，填写密钥、证书与 CA 路径；普通 BACnet/IP 无需填写。
                </div>
              </div>
            </div>
          </section>

          <section class="help-doc-section">
            <h2 class="help-doc-section__title">常见问题</h2>
            <a-collapse class="help-doc-faq" :bordered="false" expand-icon-position="right">
              <a-collapse-item header="为什么读不到值 / 显示 0?" key="1">
                先用设备厂商工具确认对象类型与实例号；检查本机/目标 IP、端口（47808）是否可达，跨网段需配置 BBMD。也可在通道页用"扫描设备"发现对象。
              </a-collapse-item>
              <a-collapse-item header="写入失败?" key="2">
                AnalogInput 等通常为只读；只有 Output / Value 类对象（如 AnalogOutput、AnalogValue）可写。写失败多为对象不支持写。
              </a-collapse-item>
              <a-collapse-item header="数据类型怎么配?" key="3">
                与设备对象属性类型一致：模拟量多为 <code>real</code>（浮点），开关量为 <code>boolean</code>，计数/整型用 <code>int</code>；再与"通用点位配置字段"的解析类型对应。
              </a-collapse-item>
            </a-collapse>
          </section>
        </template>

        <!-- OPC UA -->
        <template v-else-if="channelProtocol.includes('opc-ua')">
          <section class="help-doc-section">
            <h2 class="help-doc-section__title">协议介绍</h2>
            <p class="help-doc-section__text">
              OPC UA 是跨平台、面向服务的工业数据交换标准，基于 <strong>TCP（opc.tcp://）</strong>或 HTTPS/WebSocket。
              网关作为 <strong>OPC UA 客户端</strong>连接服务器，按 <strong>NodeID</strong> 订阅/读取节点值，安全性（认证、加密）能力强。
            </p>
          </section>

          <section class="help-doc-section">
            <h2 class="help-doc-section__title">节点类型与数据类型</h2>
            <div class="help-doc-card">
              <div v-for="item in opcuaNodeSpecs" :key="item.value" class="help-doc-row">
                <div class="help-doc-row__label">
                  <strong>{{ item.name }}</strong>
                </div>
                <code class="help-doc-row__value">{{ item.value }}</code>
              </div>
            </div>
            <div class="help-doc-card" style="margin-top:12px">
              <div v-for="item in opcuaDatatypeSpecs" :key="item.name" class="help-doc-row">
                <div class="help-doc-row__label">
                  <strong>{{ item.name }}</strong>
                </div>
                <code class="help-doc-row__value">{{ item.value }}</code>
              </div>
            </div>
          </section>

          <section class="help-doc-section">
            <h2 class="help-doc-section__title">点位地址格式（NodeID）</h2>
            <p class="help-doc-section__text">
              使用 <strong>NodeID</strong> 唯一定位节点，两种写法：
            </p>
            <div class="help-doc-card">
              <div class="help-doc-row help-doc-row--stack">
                <div class="help-doc-row__label">
                  <strong>字符串型</strong>
                  <span>ns=命名空间;s=标识</span>
                </div>
                <div class="help-doc-row__desc">
                  最常用，如 <code>ns=2;s=Channel1.Device1.Tag1</code>；标识为服务器地址空间中的字符串名。
                </div>
              </div>
              <div class="help-doc-row help-doc-row--stack">
                <div class="help-doc-row__label">
                  <strong>数值型</strong>
                  <span>ns=命名空间;i=标识</span>
                </div>
                <div class="help-doc-row__desc">
                  用整数 NodeID，如 <code>ns=2;i=12345</code>；部分服务器仅暴露数值 ID。
                </div>
              </div>
            </div>
            <p class="help-doc-example">
              推荐用 UA Expert / UaBrowser 等客户端浏览服务器地址空间，复制目标节点 NodeID 直接粘贴到"地址"栏。
            </p>
          </section>

          <section class="help-doc-section">
            <h2 class="help-doc-section__title">通道连接配置</h2>
            <div class="help-doc-card">
              <div class="help-doc-row help-doc-row--stack">
                <div class="help-doc-row__label">
                  <strong>Endpoint URL</strong>
                  <span>config.url</span>
                </div>
                <div class="help-doc-row__desc">
                  服务器端点，如 <code>opc.tcp://192.168.1.20:4840</code>。端口常为 4840，需与服务器实际端点一致（含安全模式）。
                </div>
              </div>
              <div class="help-doc-row help-doc-row--stack">
                <div class="help-doc-row__label">
                  <strong>安全策略 / 模式</strong>
                  <span>SecurityPolicy / Mode</span>
                </div>
                <div class="help-doc-row__desc">
                  None / Basic128Rsa256 等 + Sign / Sign&amp;Encrypt。若服务器要求加密，需上传客户端证书并在服务器端信任；匿名模式选 None。
                </div>
              </div>
              <div class="help-doc-row help-doc-row--stack">
                <div class="help-doc-row__label">
                  <strong>认证方式</strong>
                  <span>匿名 / 用户名密码</span>
                </div>
                <div class="help-doc-row__desc">
                  匿名连接填空即可；需登录时在服务器侧配置用户，并在网关提供凭据。
                </div>
              </div>
            </div>
          </section>

          <section class="help-doc-section">
            <h2 class="help-doc-section__title">常见问题</h2>
            <a-collapse class="help-doc-faq" :bordered="false" expand-icon-position="right">
              <a-collapse-item header="连不上服务器?" key="1">
                检查 Endpoint URL 与端口是否可达；确认安全模式（None / 加密）匹配；加密场景需在服务器端信任网关证书，且时间同步正常。
              </a-collapse-item>
              <a-collapse-item header="NodeID 怎么找?" key="2">
                用 OPC UA 客户端（UA Expert 等）浏览地址空间，找到目标节点后复制其 NodeID；注意命名空间索引 ns= 应与服务器一致。
              </a-collapse-item>
              <a-collapse-item header="读到的类型不对?" key="3">
                在"通用点位配置字段"中选择与服务器节点一致的数据类型（如 Double/Float/String），解析类型与数据类型需匹配，避免北向类型错误。
              </a-collapse-item>
            </a-collapse>
          </section>
        </template>

        <!-- S7 (Siemens) -->
        <template v-else-if="channelProtocol === 's7'">
          <section class="help-doc-section">
            <h2 class="help-doc-section__title">协议介绍</h2>
            <p class="help-doc-section__text">
              S7 指西门子 S7 系列 PLC（S7-200Smart / 1200 / 1500 / 300 / 400）的 <strong>S7 Communication</strong> 协议，基于 TCP（默认端口 102）。
              网关作为客户端，按 <strong>数据块地址</strong>读取 PLC 的 DB、M、I、Q、T、C 等存储区。
            </p>
          </section>
          <section class="help-doc-section">
            <h2 class="help-doc-section__title">点位地址格式</h2>
            <p class="help-doc-section__text">地址格式：<code>区域 + 偏移[.位]</code>，并区分字节/字/双字：</p>
            <div class="help-doc-card">
              <div class="help-doc-row help-doc-row--stack">
                <div class="help-doc-row__label"><strong>DB 区</strong></div>
                <div class="help-doc-row__desc"><code>DB1.DBD0</code> 双字(浮点/长整)、<code>DB1.DBW2</code> 字(整型)、<code>DB1.DBX0.1</code> 位、<code>DB1.DBB4</code> 字节。</div>
              </div>
              <div class="help-doc-row help-doc-row--stack">
                <div class="help-doc-row__label"><strong>M / I / Q 区</strong></div>
                <div class="help-doc-row__desc"><code>M0.0</code> 位、<code>MD0</code> 双字、<code>MW0</code> 字；<code>I0.0</code>/<code>Q0.0</code> 位、<code>ID0</code>/<code>QD0</code> 双字。</div>
              </div>
              <div class="help-doc-row help-doc-row--stack">
                <div class="help-doc-row__label"><strong>T / C</strong></div>
                <div class="help-doc-row__desc"><code>T0</code> 定时器、<code>C0</code> 计数器。</div>
              </div>
            </div>
            <p class="help-doc-example">例：读取 DB1 中从字节 0 开始的 32 位浮点 → 地址 <code>DB1.DBD0</code>，字节数 4、解析类型 FLOAT32。</p>
          </section>
          <section class="help-doc-section">
            <h2 class="help-doc-section__title">通道连接配置</h2>
            <div class="help-doc-card">
              <div class="help-doc-row help-doc-row--stack"><div class="help-doc-row__label"><strong>PLC IP（必填）</strong><span>config.ip</span></div><div class="help-doc-row__desc">PLC 的 IP，如 192.168.1.10。</div></div>
              <div class="help-doc-row help-doc-row--stack"><div class="help-doc-row__label"><strong>端口 / 型号</strong><span>port(102) / plcType</span></div><div class="help-doc-row__desc">端口默认 102；型号用于选择通信参数（S7-1200/1500 默认 Rack=0 Slot=1，S7-300/400 按硬件组态）。</div></div>
              <div class="help-doc-row help-doc-row--stack"><div class="help-doc-row__label"><strong>机架号 / 槽号</strong><span>rack / slot</span></div><div class="help-doc-row__desc">与 PLC 硬件组态一致；填错会连接失败。S7-1200 通常 0/1，S7-1500 的 CPU 在 Slot 1。</div></div>
              <div class="help-doc-row help-doc-row--stack"><div class="help-doc-row__label"><strong>连接类型 / CPU 停机保护</strong><span>connect_type / cpu_protection</span></div><div class="help-doc-row__desc">默认自动；生产环境建议开启 CPU 停机保护，避免误写导致 PLC 停机。</div></div>
              <div class="help-doc-row help-doc-row--stack"><div class="help-doc-row__label"><strong>性能参数</strong><span>timeout / max_retries / heartbeat / batch_read_max</span></div><div class="help-doc-row__desc">超时(2000ms)、重试(1)、心跳(30s)、批量读取上限(100) 可按网络与负载调整。</div></div>
            </div>
          </section>
          <section class="help-doc-section">
            <h2 class="help-doc-section__title">常见问题</h2>
            <a-collapse class="help-doc-faq" :bordered="false" expand-icon-position="right">
              <a-collapse-item header="连不上 PLC?" key="1">检查 IP、机架/槽号与硬件组态是否一致；确认端口 102 可达、防火墙放行；S7-1200/1500 需在 TIA 中勾选"允许 PUT/GET 通信"。</a-collapse-item>
              <a-collapse-item header="读到的值不对?" key="2">地址区与类型要匹配：DBD=双字浮点、DBW=字整型、DBX=位；字节数/字序按"通用点位配置字段"设置。</a-collapse-item>
              <a-collapse-item header="优化采集性能?" key="3">提高"批量读取上限"可合并请求；对高频点位适当拉长采集周期，并开启心跳保活。</a-collapse-item>
            </a-collapse>
          </section>
        </template>

        <!-- EtherNet/IP -->
        <template v-else-if="channelProtocol === 'ethernet-ip'">
          <section class="help-doc-section">
            <h2 class="help-doc-section__title">协议介绍</h2>
            <p class="help-doc-section__text">
              EtherNet/IP（简称 EIP）是罗克韦尔/Allen-Bradley 等采用的工业以太网协议，基于 <strong>TCP（端口 44818）</strong>与 CIP（通用工业协议）。
              网关作为 <strong>Explicit Messaging / 主站</strong>读取 PLC 的标签（Tag）或数据文件。
            </p>
          </section>
          <section class="help-doc-section">
            <h2 class="help-doc-section__title">点位地址格式</h2>
            <p class="help-doc-section__text">地址为 <strong>Tag 名称</strong>（标签路径），如 <code>Program:Main.MyTag</code>；ControlLogix/CompactLogix 用 <strong>Logix 模式</strong>，老款 PLC-5/SLC 用 <strong>CIP 标准模式</strong>配合数据文件地址。</p>
            <p class="help-doc-example">例：读取主例程中名为 <code>Temperature</code> 的标签 → 地址 <code>Program:Main.Temperature</code>，按标签数据类型选字节数与解析类型。</p>
          </section>
          <section class="help-doc-section">
            <h2 class="help-doc-section__title">通道连接配置</h2>
            <div class="help-doc-card">
              <div class="help-doc-row help-doc-row--stack"><div class="help-doc-row__label"><strong>PLC IP（必填）</strong><span>config.ip</span></div><div class="help-doc-row__desc">PLC 的 IP，如 192.168.1.10。</div></div>
              <div class="help-doc-row help-doc-row--stack"><div class="help-doc-row__label"><strong>端口 / 槽号</strong><span>port(44818) / slot</span></div><div class="help-doc-row__desc">端口默认 44818；槽号用于背板寻址（CompactLogix 通常 0，ControlLogix 按机架空槽）。</div></div>
              <div class="help-doc-row help-doc-row--stack"><div class="help-doc-row__label"><strong>连接类型</strong><span>connection_type</span></div><div class="help-doc-row__desc"><code>cip</code> 标准 CIP 模式 / <code>logix</code> Logix 标签模式，按 PLC 型号选择。</div></div>
              <div class="help-doc-row help-doc-row--stack"><div class="help-doc-row__label"><strong>性能参数</strong><span>timeout / max_retries / batch_read_max / min_interval</span></div><div class="help-doc-row__desc">超时(2000ms)、重试(3)、批量读取上限(50)、最小指令间隔(5ms) 可按负载调整。</div></div>
            </div>
          </section>
          <section class="help-doc-section">
            <h2 class="help-doc-section__title">常见问题</h2>
            <a-collapse class="help-doc-faq" :bordered="false" expand-icon-position="right">
              <a-collapse-item header="连不上?" key="1">检查 IP、端口 44818 可达、防火墙放行；确认 Forward Open 许可充足（连接数上限）。</a-collapse-item>
              <a-collapse-item header="标签不存在 / 读不到?" key="2">确认 Controller Scope 中的标签名（区分大小写、含程序域 <code>Program:Main.</code> 前缀）；连接类型与 PLC 型号匹配。</a-collapse-item>
              <a-collapse-item header="大数组读取慢?" key="3">提高"批量读取上限"合并请求；对结构体/数组标签按其成员逐一建点并合理设置最小间隔。</a-collapse-item>
            </a-collapse>
          </section>
        </template>

        <!-- Mitsubishi SLMP / MC -->
        <template v-else-if="channelProtocol === 'mitsubishi-slmp'">
          <section class="help-doc-section">
            <h2 class="help-doc-section__title">协议介绍</h2>
            <p class="help-doc-section__text">
              三菱 SLMP（Seamless Message Protocol）/ MC 协议，用于 Q、L、iQ-R、iQ-F 等系列 PLC 的以太网通信，默认 <strong>TCP 端口 5000</strong>。
              网关作为客户端，按 <strong>软元件地址</strong>读写。
            </p>
          </section>
          <section class="help-doc-section">
            <h2 class="help-doc-section__title">点位地址格式</h2>
            <p class="help-doc-section__text">格式：<code>软元件字母 + 编号[.位][.L]</code>，如 <code>D100</code>、<code>M0</code>、<code>X0</code>、<code>Y0</code>、<code>D20.2</code>（第 2 位）、<code>D100.16L</code>（32 位长整）。</p>
            <div class="help-doc-card">
              <div class="help-doc-row help-doc-row--stack"><div class="help-doc-row__label"><strong>数据寄存器</strong><span>D</span></div><div class="help-doc-row__desc">16 位字，加 <code>.L</code> 表示 32 位（如 D100.16L 中 16=起始位、L=长整型）。</div></div>
              <div class="help-doc-row help-doc-row--stack"><div class="help-doc-row__label"><strong>辅助继电器 / 输入输出</strong><span>M / X / Y</span></div><div class="help-doc-row__desc">位元件，后接位号；读取位元件需注意是位还是字。</div></div>
            </div>
            <p class="help-doc-example">例：读取 D100 开始的 32 位浮点 → 地址 <code>D100</code>，字节数 4、解析类型 FLOAT32。</p>
          </section>
          <section class="help-doc-section">
            <h2 class="help-doc-section__title">通道连接配置</h2>
            <div class="help-doc-card">
              <div class="help-doc-row help-doc-row--stack"><div class="help-doc-row__label"><strong>PLC IP（必填）</strong><span>config.ip</span></div><div class="help-doc-row__desc">PLC 的 IP，如 192.168.1.10。</div></div>
              <div class="help-doc-row help-doc-row--stack"><div class="help-doc-row__label"><strong>端口 / 帧类型</strong><span>port(5000) / frame_type</span></div><div class="help-doc-row__desc">端口默认 5000；帧类型 3E（Q/L/iQ-R/iQ-F 常用）。</div></div>
              <div class="help-doc-row help-doc-row--stack"><div class="help-doc-row__label"><strong>站号 / 网络号 / PC 编号</strong><span>station_no / network_no / pc_no</span></div><div class="help-doc-row__desc">默认 0 / 0 / 255；多网络/多站时按 PLC 网络参数填写。</div></div>
              <div class="help-doc-row help-doc-row--stack"><div class="help-doc-row__label"><strong>性能参数</strong><span>timeout / max_retries / batch_read_max</span></div><div class="help-doc-row__desc">超时(3000ms)、重试(2)、批量读取上限(64)。</div></div>
            </div>
          </section>
          <section class="help-doc-section">
            <h2 class="help-doc-section__title">常见问题</h2>
            <a-collapse class="help-doc-faq" :bordered="false" expand-icon-position="right">
              <a-collapse-item header="连不上?" key="1">检查 IP、端口 5000 可达、防火墙放行；确认帧类型与 PLC 实际一致（多为 3E）。</a-collapse-item>
              <a-collapse-item header="地址/值不对?" key="2">软元件字母须与设备一致（D/M/X/Y）；32 位值加 <code>.L</code> 后缀并用 4 字节解析。</a-collapse-item>
            </a-collapse>
          </section>
        </template>

        <!-- Omron FINS -->
        <template v-else-if="channelProtocol === 'omron-fins'">
          <section class="help-doc-section">
            <h2 class="help-doc-section__title">协议介绍</h2>
            <p class="help-doc-section__text">
              欧姆龙 FINS（Factory Interface Network Service）协议，支持 <strong>TCP / UDP</strong>（默认端口 9600），用于 CP/CJ/CS/NJ 等系列 PLC。
              网关作为客户端，按 <strong>存储区地址</strong>读写。
            </p>
          </section>
          <section class="help-doc-section">
            <h2 class="help-doc-section__title">点位地址格式</h2>
            <p class="help-doc-section__text">格式：<code>区域.字[.位]</code>，如 <code>CIO1.2</code>、<code>D100</code>、<code>W3.4</code>、<code>EM10.100</code>。</p>
            <div class="help-doc-card">
              <div class="help-doc-row help-doc-row--stack"><div class="help-doc-row__label"><strong>CIO / D / W / EM</strong></div><div class="help-doc-row__desc">CIO 为 I/O 与内部继电器区；D 为数据存储器；W 为工作区；EM 为扩展数据内存（带区号）。</div></div>
              <div class="help-doc-row help-doc-row--stack"><div class="help-doc-row__label"><strong>.位 后缀</strong></div><div class="help-doc-row__desc">在字地址后加 <code>.位号</code>（0–15）表示读取该字的某个位。</div></div>
            </div>
            <p class="help-doc-example">例：读取 D100 开始的 32 位浮点 → 地址 <code>D100</code>，字节数 4、解析类型 FLOAT32。</p>
          </section>
          <section class="help-doc-section">
            <h2 class="help-doc-section__title">通道连接配置</h2>
            <div class="help-doc-card">
              <div class="help-doc-row help-doc-row--stack"><div class="help-doc-row__label"><strong>PLC IP（必填）</strong><span>config.ip</span></div><div class="help-doc-row__desc">PLC 的 IP，如 192.168.1.100。</div></div>
              <div class="help-doc-row help-doc-row--stack"><div class="help-doc-row__label"><strong>端口 / 传输模式</strong><span>port(9600) / mode</span></div><div class="help-doc-row__desc">端口默认 9600；模式 TCP 或 UDP（UDP 需配置本地端口）。</div></div>
              <div class="help-doc-row help-doc-row--stack"><div class="help-doc-row__label"><strong>PLC 型号</strong><span>model</span></div><div class="help-doc-row__desc">CP1E / CP1H / CJ / CS / NJ，影响地址解析与命令格式。</div></div>
              <div class="help-doc-row help-doc-row--stack"><div class="help-doc-row__label"><strong>FINS 节点地址</strong><span>src/dst network·node·unit</span></div><div class="help-doc-row__desc">源/目标网络、节点、单元地址；<strong>目标节点常填 PLC IP 末段</strong>；多段路由需与设备 FINS 网络一致。</div></div>
              <div class="help-doc-row help-doc-row--stack"><div class="help-doc-row__label"><strong>性能参数</strong><span>timeout / max_retries / heartbeat / maxFrameLength</span></div><div class="help-doc-row__desc">超时(3000ms)、重试(3)、心跳(30s)、批量读取字数上限(64)。</div></div>
            </div>
          </section>
          <section class="help-doc-section">
            <h2 class="help-doc-section__title">常见问题</h2>
            <a-collapse class="help-doc-faq" :bordered="false" expand-icon-position="right">
              <a-collapse-item header="连不上?" key="1">检查 IP、端口 9600 可达；UDP 模式需填本地端口；FINS 节点路由（目标节点=IP 末段）是否正确。</a-collapse-item>
              <a-collapse-item header="NJ 系列推荐?" key="2">NJ/NX 系列更常用 EtherNet/IP（标签）方式接入；用 FINS 时需确认 CPU 单元号与节点映射。</a-collapse-item>
            </a-collapse>
          </section>
        </template>

        <!-- KNXnet/IP -->
        <template v-else-if="channelProtocol === 'knxnet-ip'">
          <section class="help-doc-section">
            <h2 class="help-doc-section__title">协议介绍</h2>
            <p class="help-doc-section__text">
              KNXnet/IP 是智能建筑总线 KNX 的 IP 隧道协议，基于 <strong>TCP / UDP（端口 3671）</strong>。
              网关作为 <strong>隧道客户端</strong>连接 KNX 网关/路由器，读写<strong>组地址（Group Address）</strong>。
            </p>
          </section>
          <section class="help-doc-section">
            <h2 class="help-doc-section__title">点位地址格式</h2>
            <p class="help-doc-section__text">组地址格式 <code>主/中/子</code>，如 <code>1/2/3</code>；可选 <code>,区域.线.设备,位</code> 指定设备与位，如 <code>1/2/3,1.1.1,2</code>。</p>
            <p class="help-doc-example">例：读取"照明/楼层1/开关1"组地址 → 地址 <code>1/2/3</code>，按 DPT 选数据类型（如 1.001 开关量、9.001 温度）。</p>
          </section>
          <section class="help-doc-section">
            <h2 class="help-doc-section__title">通道连接配置</h2>
            <div class="help-doc-card">
              <div class="help-doc-row help-doc-row--stack"><div class="help-doc-row__label"><strong>网关 IP</strong><span>config.ip</span></div><div class="help-doc-row__desc">KNX/IP 网关或路由器的 IP；启用自动发现时可留空。</div></div>
              <div class="help-doc-row help-doc-row--stack"><div class="help-doc-row__label"><strong>端口 / 传输模式</strong><span>port(3671) / mode</span></div><div class="help-doc-row__desc">端口默认 3671；模式 UDP（常用）或 TCP。</div></div>
              <div class="help-doc-row help-doc-row--stack"><div class="help-doc-row__label"><strong>网关发现 (SEARCH)</strong><span>discovery / discovery_multicast</span></div><div class="help-doc-row__desc">开启后广播搜索局域网内 KNX/IP 网关（多播 224.0.23.12:3671），自动填充 IP。</div></div>
              <div class="help-doc-row help-doc-row--stack"><div class="help-doc-row__label"><strong>性能参数</strong><span>timeout / max_retries / heartbeat / local_ip</span></div><div class="help-doc-row__desc">超时(3000ms)、重试(3)、心跳(60s)；多网卡时指定本地 IP。</div></div>
            </div>
          </section>
          <section class="help-doc-section">
            <h2 class="help-doc-section__title">常见问题</h2>
            <a-collapse class="help-doc-faq" :bordered="false" expand-icon-position="right">
              <a-collapse-item header="连不上?" key="1">检查网关 IP、端口 3671 可达；组地址格式必须为 主/中/子；多网卡指定 local_ip。</a-collapse-item>
              <a-collapse-item header="只能收不能发 / 反之?" key="2">确认组地址方向（读/写）与 KNX 侧关联；隧道连接数有限，必要时重启网关释放隧道。</a-collapse-item>
            </a-collapse>
          </section>
        </template>

        <!-- Profinet IO -->
        <template v-else-if="channelProtocol === 'profinet-io'">
          <section class="help-doc-section">
            <h2 class="help-doc-section__title">协议介绍</h2>
            <p class="help-doc-section__text">
              PROFINET IO 是基于工业以太网的现场总线，网关作为 <strong>IO 控制器（Controller）</strong>连接 IO 设备（如远程 IO 站、变频器）。
              需绑定<strong>物理网卡</strong>并在设备 GSDML 中映射过程数据（建议裸机部署）。
            </p>
          </section>
          <section class="help-doc-section">
            <h2 class="help-doc-section__title">点位地址格式</h2>
            <p class="help-doc-section__text">格式：<code>SLOT:SUB_SLOT:INDEX[.BIT][#ENDIAN]</code>。</p>
            <div class="help-doc-card">
              <div class="help-doc-row help-doc-row--stack"><div class="help-doc-row__label"><strong>槽:子槽:索引</strong></div><div class="help-doc-row__desc"><code>3:1:0</code> 表示第 3 槽、子槽 1、索引 0 的过程数据。</div></div>
              <div class="help-doc-row help-doc-row--stack"><div class="help-doc-row__label"><strong>.BIT / #ENDIAN</strong></div><div class="help-doc-row__desc"><code>3:2:5.3</code> 取该字第 3 位；<code>#LE</code> 指定小端（Little Endian）。</div></div>
            </div>
            <p class="help-doc-example">例：读取某模块输入双字 → 地址 <code>3:1:0</code>（int16）/ <code>3:2:10</code>（float）/ <code>3:2:5.3</code>（bit）。具体偏移参考设备 GSDML 与模块映射。</p>
          </section>
          <section class="help-doc-section">
            <h2 class="help-doc-section__title">通道连接配置</h2>
            <div class="help-doc-card">
              <div class="help-doc-row help-doc-row--stack"><div class="help-doc-row__label"><strong>本地网口（必填）</strong><span>local_interface</span></div><div class="help-doc-row__desc">绑定用于 PROFINET 通信的物理网卡（如 eth0）；需裸机部署、实时内核更佳。</div></div>
              <div class="help-doc-row help-doc-row--stack"><div class="help-doc-row__label"><strong>模拟模式</strong><span>simulation</span></div><div class="help-doc-row__desc">开启后使用内置模拟器，<strong>无需真实 PROFINET IO 设备</strong>即可测试点位与流程。</div></div>
              <div class="help-doc-row help-doc-row--stack"><div class="help-doc-row__label"><strong>性能参数</strong><span>timeout / max_retries / heartbeat</span></div><div class="help-doc-row__desc">超时(3000ms)、重试(3)、心跳(30s)。</div></div>
            </div>
          </section>
          <section class="help-doc-section">
            <h2 class="help-doc-section__title">常见问题</h2>
            <a-collapse class="help-doc-faq" :bordered="false" expand-icon-position="right">
              <a-collapse-item header="要求什么环境?" key="1">PROFINET-IO 需要绑定真实网卡并通常由裸机/实时系统运行；开发阶段可开"模拟模式"验证。</a-collapse-item>
              <a-collapse-item header="地址怎么对应?" key="2">按设备 GSDML 中模块的过程数据偏移填写 SLOT:SUB_SLOT:INDEX；位与字节序（#LE）需与设备一致。</a-collapse-item>
            </a-collapse>
          </section>
        </template>

        <!-- IEC 60870-5-104 -->
        <template v-else-if="channelProtocol === 'iec60870-5-104'">
          <section class="help-doc-section">
            <h2 class="help-doc-section__title">协议介绍</h2>
            <p class="help-doc-section__text">
              IEC 60870-5-104（简称 104）是电力、轨道交通领域的远动通信协议，基于 <strong>TCP（端口 2404）</strong>。
              网关作为 <strong>控制站（主站）</strong>连接被控站（RTU/IED），按 <strong>IOA（信息对象地址）</strong>与 <strong>类型标识（TypeID）</strong>收发数据。
            </p>
          </section>
          <section class="help-doc-section">
            <h2 class="help-doc-section__title">点位地址格式</h2>
            <p class="help-doc-section__text"><strong>IOA</strong> 范围 0–65535；点位"组/类型"字段填写 <strong>TypeID</strong>，如 <code>M_ME_NC_1</code>（短浮点测量值）。</p>
            <div class="help-doc-card">
              <div class="help-doc-row help-doc-row--stack"><div class="help-doc-row__label"><strong>常用类型</strong></div><div class="help-doc-row__desc"><code>M_SP_NA_1</code> 单点遥信、<code>M_ME_NA_1</code> 归一化遥测、<code>M_ME_NC_1</code> 短浮点遥测、<code>M_IT_NA_1</code> 累计量。</div></div>
              <div class="help-doc-row help-doc-row--stack"><div class="help-doc-row__label"><strong>IOA 与类型匹配</strong></div><div class="help-doc-row__desc">每个点位须指定正确的 TypeID，否则解析出的类型/长度错误。</div></div>
            </div>
          </section>
          <section class="help-doc-section">
            <h2 class="help-doc-section__title">通道连接配置</h2>
            <div class="help-doc-card">
              <div class="help-doc-row help-doc-row--stack"><div class="help-doc-row__label"><strong>设备 IP（必填）</strong><span>config.ip</span></div><div class="help-doc-row__desc">被控站 IP，如 192.168.1.100。</div></div>
              <div class="help-doc-row help-doc-row--stack"><div class="help-doc-row__label"><strong>端口 / 公共地址</strong><span>port(2404) / commonAddress</span></div><div class="help-doc-row__desc">端口默认 2404；公共地址 CA（装置地址）需与对端一致（通常 1）。</div></div>
              <div class="help-doc-row help-doc-row--stack"><div class="help-doc-row__label"><strong>总召唤间隔</strong><span>generalCallInterval</span></div><div class="help-doc-row__desc">定期发起总召唤刷新全量数据，默认 300 秒（0 表示不主动召唤）。</div></div>
              <div class="help-doc-row help-doc-row--stack"><div class="help-doc-row__label"><strong>协议定时器</strong><span>T0/T1/T2/T3</span></div><div class="help-doc-row__desc">T0=建立超时(10s)、T1=确认超时(15s)、T2=空闲确认(10s)、T3=空闲连接测试(20s)；网络差时可适当调大。</div></div>
            </div>
          </section>
          <section class="help-doc-section">
            <h2 class="help-doc-section__title">常见问题</h2>
            <a-collapse class="help-doc-faq" :bordered="false" expand-icon-position="right">
              <a-collapse-item header="连不上?" key="1">检查 IP、端口 2404 可达；公共地址 CA 与对端一致；必要时降低 T1/T2 或开启总召唤。</a-collapse-item>
              <a-collapse-item header="数据错乱 / 读不到?" key="2">确认每个点位的 TypeID 与 IOA 映射正确（浮点用 M_ME_NC_1，遥信用 M_SP_NA_1）。</a-collapse-item>
            </a-collapse>
          </section>
        </template>

        <!-- SNMP -->
        <template v-else-if="channelProtocol === 'snmp'">
          <section class="help-doc-section">
            <h2 class="help-doc-section__title">协议介绍</h2>
            <p class="help-doc-section__text">
              SNMP 是网络设备/UPS/工业网关的管理协议，基于 <strong>UDP（端口 161）</strong>。网关作为 <strong>管理站</strong>按 <strong>OID</strong> 读取设备 MIB 中的标量。
            </p>
          </section>
          <section class="help-doc-section">
            <h2 class="help-doc-section__title">点位地址格式</h2>
            <p class="help-doc-section__text">地址格式 <code>community|OID</code>（v2c）或 <code>securityName|OID</code>（v3）。</p>
            <div class="help-doc-card">
              <div class="help-doc-row help-doc-row--stack"><div class="help-doc-row__label"><strong>OID</strong></div><div class="help-doc-row__desc">点位的叶子节点，如 <code>1.3.6.1.2.1.1.1.0</code>（sysDescr）。可在设备 MIB 中查到。</div></div>
              <div class="help-doc-row help-doc-row--stack"><div class="help-doc-row__label"><strong>community / 安全名</strong></div><div class="help-doc-row__desc">v2c 用团体名（如 public）；v3 用安全名称（用户名）。</div></div>
            </div>
            <p class="help-doc-example">例：读取系统描述 → 地址 <code>public|1.3.6.1.2.1.1.1.0</code>；v3 为 <code>admin|1.3.6.1.2.1.1.1.0</code>。</p>
          </section>
          <section class="help-doc-section">
            <h2 class="help-doc-section__title">通道连接配置</h2>
            <div class="help-doc-card">
              <div class="help-doc-row help-doc-row--stack"><div class="help-doc-row__label"><strong>设备 IP（必填）</strong><span>config.ip</span></div><div class="help-doc-row__desc">被管设备 IP，如 192.168.1.1。</div></div>
              <div class="help-doc-row help-doc-row--stack"><div class="help-doc-row__label"><strong>端口 / 版本</strong><span>port(161) / snmpVersion</span></div><div class="help-doc-row__desc">端口默认 161；版本 v2c（团体名）或 v3（用户名+安全）。</div></div>
              <div class="help-doc-row help-doc-row--stack"><div class="help-doc-row__label"><strong>v2c 团体名</strong><span>community</span></div><div class="help-doc-row__desc">如 public；需与设备 SNMP 团体配置一致。</div></div>
              <div class="help-doc-row help-doc-row--stack"><div class="help-doc-row__label"><strong>v3 安全</strong><span>securityName / securityLevel / auth·priv</span></div><div class="help-doc-row__desc">安全名、级别(noAuthNoPriv/authNoPriv/authPriv)、认证协议(MD5/SHA*)、加密协议(DES/AES*)与对应密码。</div></div>
              <div class="help-doc-row help-doc-row--stack"><div class="help-doc-row__label"><strong>性能参数</strong><span>timeout / retries / maxBulkSize / sendInterval</span></div><div class="help-doc-row__desc">超时(3000ms)、重试(3)、GETBULK 数量(10)、发送间隔(100ms)。</div></div>
            </div>
          </section>
          <section class="help-doc-section">
            <h2 class="help-doc-section__title">常见问题</h2>
            <a-collapse class="help-doc-faq" :bordered="false" expand-icon-position="right">
              <a-collapse-item header="读不到值?" key="1">确认 IP、端口 161 可达；community / 安全名与设备一致；OID 为完整叶子节点（末尾带 .0）。</a-collapse-item>
              <a-collapse-item header="v3 连不上?" key="2">核对安全级别、认证/加密协议与密码完全匹配；设备需已创建对应 SNMPv3 用户。</a-collapse-item>
            </a-collapse>
          </section>
        </template>

        <!-- DLT645 -->
        <template v-else-if="channelProtocol === 'dlt645'">
          <section class="help-doc-section">
            <h2 class="help-doc-section__title">协议介绍</h2>
            <p class="help-doc-section__text">
              DLT645 是中国电力行业多功能电能表通信协议，支持 <strong>串口（RS-485）</strong>与 <strong>TCP（端口 8001）</strong>两种连接方式，常用于智能电表、水表、气表。
            </p>
          </section>
          <section class="help-doc-section">
            <h2 class="help-doc-section__title">点位地址格式</h2>
            <p class="help-doc-section__text">地址为 <strong>数据标识（Data Identifier）</strong>，格式 <code>XX-XX-XX-XX</code>，如 <code>02-01-01-00</code>（组合有功总电能）、<code>02-02-01-00</code>（正向有功总电能）等。具体标识见 DLT645-2007 / 1997 附录。</p>
            <p class="help-doc-example">例：读取电度 → 地址 <code>02-01-01-00</code>，按表计返回格式选字节数与解析类型（多为 BCD 或整型）。</p>
          </section>
          <section class="help-doc-section">
            <h2 class="help-doc-section__title">通道连接配置</h2>
            <div class="help-doc-card">
              <div class="help-doc-row help-doc-row--stack"><div class="help-doc-row__label"><strong>连接方式</strong><span>connectionType</span></div><div class="help-doc-row__desc"><code>serial</code> 串口（RS-485）或 <code>tcp</code> 网络（端口 8001）。</div></div>
              <div class="help-doc-row help-doc-row--stack"><div class="help-doc-row__label"><strong>串口参数（serial）</strong><span>port / baudRate / dataBits / stopBits / parity</span></div><div class="help-doc-row__desc">串口设备（如 /dev/ttyS1）、波特率(默认 9600)、数据位 8、停止位 1、校验(无/偶/奇)；须与表计一致。</div></div>
              <div class="help-doc-row help-doc-row--stack"><div class="help-doc-row__label"><strong>网络参数（tcp）</strong><span>ip / port / timeout</span></div><div class="help-doc-row__desc">设备 IP 与端口(8001)、超时。</div></div>
            </div>
          </section>
          <section class="help-doc-section">
            <h2 class="help-doc-section__title">常见问题</h2>
            <a-collapse class="help-doc-faq" :bordered="false" expand-icon-position="right">
              <a-collapse-item header="读不到?" key="1">串口方式检查波特率/校验/停止位与表计一致，确认串口设备存在且进程有读权限；网络方式检查 IP/端口 8001 可达。</a-collapse-item>
              <a-collapse-item header="数据标识顺序?" key="2">DLT645-2007 与 1997 的数据标识编码不同，按表计版本选择；标识需为完整的 4 字节 XX-XX-XX-XX。</a-collapse-item>
            </a-collapse>
          </section>
        </template>

        <!-- EtherCAT -->
        <template v-else-if="channelProtocol === 'ethercat'">
          <section class="help-doc-section">
            <h2 class="help-doc-section__title">协议介绍</h2>
            <p class="help-doc-section__text">
              EtherCAT 是高性能实时工业以太网，网关作为 <strong>主站（Master）</strong>通过<strong>过程数据（PDO）</strong>周期性读写从站。需绑定真实网卡（建议裸机 + 实时内核）。
            </p>
          </section>
          <section class="help-doc-section">
            <h2 class="help-doc-section__title">点位地址说明</h2>
            <p class="help-doc-section__text">点位按从站的 <strong>PDO 映射（过程数据偏移）</strong>配置，通常配合设备 ESI 描述与物理拓扑；每个 PDO 条目对应一个过程变量（位/字/双字）。</p>
            <p class="help-doc-example">配置时参考从站 ESI 中的 PDO 映射表，将变量偏移填入点位地址，并按其数据类型设置字节数与解析类型。</p>
          </section>
          <section class="help-doc-section">
            <h2 class="help-doc-section__title">通道连接配置</h2>
            <div class="help-doc-card">
              <div class="help-doc-row help-doc-row--stack"><div class="help-doc-row__label"><strong>网络接口（必填）</strong><span>local_interface</span></div><div class="help-doc-row__desc">绑定真实网卡（如 eth0、enp2s0）或 <code>lo</code>（模拟模式）。</div></div>
              <div class="help-doc-row help-doc-row--stack"><div class="help-doc-row__label"><strong>周期时间</strong><span>cycle_time_us</span></div><div class="help-doc-row__desc">通信周期（微秒），默认 1000；越小实时性越高，但受硬件/系统负载限制。</div></div>
              <div class="help-doc-row help-doc-row--stack"><div class="help-doc-row__label"><strong>模拟模式</strong><span>simulation</span></div><div class="help-doc-row__desc">开启后使用内置模拟器，<strong>无需真实 EtherCAT 硬件</strong>即可测试点位与流程。</div></div>
              <div class="help-doc-row help-doc-row--stack"><div class="help-doc-row__label"><strong>性能参数</strong><span>timeout / max_retries</span></div><div class="help-doc-row__desc">超时(3000ms)、重试(3)。</div></div>
            </div>
          </section>
          <section class="help-doc-section">
            <h2 class="help-doc-section__title">常见问题</h2>
            <a-collapse class="help-doc-faq" :bordered="false" expand-icon-position="right">
              <a-collapse-item header="需要什么环境?" key="1">EtherCAT 主站通常需裸机 + 实时内核并绑定真实网卡；开发阶段可开"模拟模式"验证流程。</a-collapse-item>
              <a-collapse-item header="点位对应不上?" key="2">按从站 ESI 的 PDO 映射表确认过程数据偏移与数据类型；不同从站映射顺序不同。</a-collapse-item>
            </a-collapse>
          </section>
        </template>

        <!-- Generic fallback -->
        <template v-else>
          <section class="help-doc-section">
            <h2 class="help-doc-section__title">协议介绍</h2>
            <p class="help-doc-section__text">
              当前协议为 {{ formatProtocolTag(channelProtocol) }}，请参考设备手册了解详细配置规范。
            </p>
          </section>

          <section class="help-doc-section">
            <h2 class="help-doc-section__title">数据格式</h2>
            <div class="help-doc-card">
              <div v-for="item in genericDatatypeSpecs" :key="item.name" class="help-doc-row">
                <div class="help-doc-row__label">
                  <strong>{{ item.name }}</strong>
                </div>
                <code class="help-doc-row__value">{{ item.value }}</code>
              </div>
            </div>
          </section>

          <section class="help-doc-section">
            <h2 class="help-doc-section__title">常见问题</h2>
            <a-collapse class="help-doc-faq" :bordered="false" expand-icon-position="right">
              <a-collapse-item header="为什么数值显示为 0?" key="1">
                请检查设备通讯状态及配置参数是否正确。
              </a-collapse-item>
              <a-collapse-item header="如何配置数据类型?" key="2">
                选择与设备端一致的数据类型。
              </a-collapse-item>
              <a-collapse-item header="如何排查通讯问题?" key="3">
                检查网络连接、设备电源、配置参数和设备状态。
              </a-collapse-item>
            </a-collapse>
          </section>
        </template>
      </div>
    </article>
  </a-drawer>
</template>

<script setup>
import { formatProtocolTag } from '@/utils/protocolLabel'

defineProps({
  visible: { type: Boolean, default: false },
  channelProtocol: { type: String, default: '' },
})

const emit = defineEmits(['update:visible', 'cancel'])

const modbusRegisterSpecs = [
  { name: '保持寄存器', meta: 'Holding Register', range: '40001 – 49999' },
  { name: '输入寄存器', meta: 'Input Register', range: '30001 – 39999' },
  { name: '线圈', meta: 'Coil', range: '00001 – 09999' },
  { name: '离散输入', meta: 'Discrete Input', range: '10001 – 19999' },
]

const modbusParseTypeMap = [
  { parse: 'BIT', datatype: 'bool / int8', usage: '单 bit 开关量（线圈、离散输入常用）' },
  { parse: 'UINT8 / INT8', datatype: 'uint8 / int8', usage: '1 字节有/无符号整数（字节型点位）' },
  { parse: 'UINT16 / INT16', datatype: 'uint16 / int16', usage: '1 寄存器（16 位）整数' },
  { parse: 'UINT16_SWAP / INT16_SWAP', datatype: 'uint16 / int16', usage: '同上，但字内字节交换（AB ↔ BA）' },
  { parse: 'BCD16', datatype: 'uint16', usage: '16 位 BCD 码，如 0x1234 解析为 1234' },
  { parse: 'FLOAT16', datatype: 'float32', usage: '半精度浮点（较少见）' },
  { parse: 'UINT32 / INT32', datatype: 'uint32 / int32', usage: '2 寄存器（32 位）长整型' },
  { parse: 'FLOAT32', datatype: 'float32', usage: '2 寄存器（32 位）单精度浮点' },
  { parse: 'FLOAT32_SWAP', datatype: 'float32', usage: '同上，字内字节交换' },
  { parse: 'BCD32', datatype: 'uint32', usage: '32 位 BCD 码' },
  { parse: 'UINT64 / INT64 / FLOAT64', datatype: 'uint64 / int64 / float64', usage: '4 寄存器（64 位）整型或双精度浮点' },
  { parse: 'STRING', datatype: 'string', usage: '按字节顺序拼接为 ASCII 字符串' },
]

const bacnetObjectSpecs = [
  { name: '模拟输入', value: 'AnalogInput' },
  { name: '模拟输出', value: 'AnalogOutput' },
  { name: '模拟值', value: 'AnalogValue' },
  { name: '二进制输入', value: 'BinaryInput' },
  { name: '二进制输出', value: 'BinaryOutput' },
  { name: '二进制值', value: 'BinaryValue' },
]

const bacnetDatatypeSpecs = [
  { name: '整数', value: 'int' },
  { name: '浮点数', value: 'real' },
  { name: '字符串', value: 'characterstring' },
  { name: '布尔值', value: 'boolean' },
]

const opcuaNodeSpecs = [
  { name: '变量', value: 'Variable' },
  { name: '对象', value: 'Object' },
  { name: '方法', value: 'Method' },
  { name: '引用', value: 'Reference' },
]

const opcuaDatatypeSpecs = [
  { name: '布尔值', value: 'Boolean' },
  { name: '整数', value: 'Int16, Int32, Int64' },
  { name: '浮点数', value: 'Float, Double' },
  { name: '字符串', value: 'String' },
]

const genericDatatypeSpecs = [
  { name: '整数', value: 'int, uint' },
  { name: '浮点数', value: 'float, double' },
  { name: '字符串', value: 'string' },
  { name: '布尔值', value: 'bool' },
]

const onCancel = () => {
  emit('update:visible', false)
  emit('cancel')
}
</script>

<style scoped>
/* v3.0 — src/styles/help-drawer.css */
</style>
