<template>
  <article class="help-doc">
    <header class="help-doc__hero">
      <span class="protocol-tag protocol-tag--accent">EtherCAT</span>
      <p class="help-doc__lead">
        EtherCAT 工业以太网现场总线，edgeCore 作为 EtherCAT 主站通过 UDP 多播（端口 0x88A4）对从站进行 PDO 过程数据采集与 CoE SDO 参数访问。
      </p>
    </header>
    <div class="help-doc__sections">
      <ChannelHelpBlock title="适用场景" text="Beckhoff 及兼容 EtherCAT 的伺服驱动器、IO 耦合器、传感器模块的数据采集与反控。" />
      <ChannelHelpBlock title="部署注意">
        <p class="help-doc__text">
          EtherCAT 主站基于原始以太网帧（UDP 多播），需将 edgeCore 部署在物理网关上并绑定真实网卡，不支持 Docker 或虚拟机环境。实时性要求高的场景需评估网卡白名单与内核实时补丁。
        </p>
      </ChannelHelpBlock>
      <ChannelHelpBlock title="通道配置">
        <ChannelHelpParamList :items="channelParams" />
      </ChannelHelpBlock>
      <ChannelHelpBlock title="设备配置">
        <ChannelHelpParamList :items="deviceParams" />
      </ChannelHelpBlock>
      <ChannelHelpBlock title="点位地址格式">
        <ChannelHelpParamList :items="addressFormat" />
      </ChannelHelpBlock>
      <ChannelHelpBlock title="地址示例">
        <ChannelHelpParamList :items="addressExamples" />
      </ChannelHelpBlock>
      <ChannelHelpBlock title="支持的数据类型">
        <p class="help-doc__text">INT8、UINT8、INT16、UINT16、INT32、UINT32、INT64、UINT64、FLOAT、DOUBLE、BIT</p>
      </ChannelHelpBlock>
      <ChannelHelpBlock title="配置步骤">
        <ol class="help-doc-steps">
          <li>创建通道，协议选择 EtherCAT，填写本地网口名称（如 eth0）。</li>
          <li>添加从站设备，填写从站位置（1..N）、厂商 ID、产品代码及 PDO 长度。</li>
          <li>设置采集间隔，进入点位页添加点位，地址格式为 POSITION:Tx|Rx:OFFSET 或 SDO 格式。</li>
          <li>开发测试时可启用模拟模式（simulation）无需真实设备与网卡。</li>
        </ol>
      </ChannelHelpBlock>
      <ChannelHelpBlock title="常见问题">
        <ChannelHelpFaq :items="faqItems" />
      </ChannelHelpBlock>
    </div>
  </article>
</template>

<script setup>
import ChannelHelpBlock from './ChannelHelpBlock.vue'
import ChannelHelpParamList from './ChannelHelpParamList.vue'
import ChannelHelpFaq from './ChannelHelpFaq.vue'

const channelParams = [
  { label: '本地网口', example: 'eth0', desc: '绑定用于 EtherCAT 通信的物理网卡（必填）' },
  { label: 'PDO 周期', example: 'cycle_time_us: 1000', desc: 'PDO 交换周期（微秒），默认 1000（1ms）' },
  { label: '超时', example: 'timeout: 3000', desc: 'SDO / 状态切换超时（毫秒），默认 3000' },
  { label: '最大重试', example: 'max_retries: 3', desc: '链路异常重试次数' },
  { label: '模拟模式', example: 'simulation: true', desc: '无真实设备/网卡时用于开发测试' },
]

const deviceParams = [
  { label: '从站位置', example: '1', desc: '从站在总线上的物理位置（1..N），必填' },
  { label: '从站别名', example: '0', desc: '可选别名地址' },
  { label: '厂商 ID', example: '0x00000002', desc: '从站 Vendor ID（十六进制）' },
  { label: '产品代码', example: '0x07D43052', desc: '从站 Product Code（十六进制）' },
  { label: '输入 PDO 长度', example: 'tx_pdo_size: 16', desc: 'TxPDO（输入）映像区字节数' },
  { label: '输出 PDO 长度', example: 'rx_pdo_size: 8', desc: 'RxPDO（输出）映像区字节数' },
  { label: '运行模式', example: 'pdo', desc: 'pdo（过程数据，默认）或 sdo（参数访问）' },
]

const addressFormat = [
  { label: 'PDO 格式', example: 'POSITION:Tx|Rx:OFFSET[.BIT][#ENDIAN]', desc: '过程数据地址' },
  { label: 'SDO 格式', example: 'POSITION:SDO:0xINDEX:0xSUBINDEX[#ENDIAN]', desc: 'CoE 对象字典地址' },
  { label: 'POSITION', desc: '从站位置（1..N）' },
  { label: 'Tx / Rx', desc: 'Tx=输入（主站读），Rx=输出（主站写）' },
  { label: 'OFFSET', desc: 'PDO 映像区内字节偏移（从 0 开始）' },
  { label: '.BIT', desc: '可选，位偏移 0–7' },
  { label: '#ENDIAN', desc: '可选，#BE（默认）或 #LE' },
]

const addressExamples = [
  { label: 'int16', example: '1:Tx:0', desc: '1 号从站 TxPDO 第 0,1 字节' },
  { label: 'bit', example: '1:Tx:2.3', desc: '1 号从站 TxPDO 第 2 字节第 3 位' },
  { label: 'uint32', example: '2:Rx:4', desc: '2 号从站 RxPDO 第 4–7 字节（反控）' },
  { label: 'float', example: '3:Tx:10', desc: '3 号从站 TxPDO 第 10–13 字节' },
  { label: 'uint16 SDO', example: '1:SDO:0x6041:0', desc: '1 号从站 CiA402 状态字' },
  { label: 'int32 SDO', example: '1:SDO:0x6064:0', desc: '1 号从站实际位置值' },
]

const faqItems = [
  { question: '连接失败？', answer: '确认网卡名称正确、从站已上电、菊花链连接正常，且网卡支持 UDP 多播（239.0.0.1:0x88A4）。' },
  { question: '读取值为 Bad？', answer: '检查从站位置与地址格式是否匹配，确认 PDO 长度配置正确，数据类型与字节长度一致。' },
  { question: 'Docker 中无法通信？', answer: 'EtherCAT 主站需直接访问物理网卡，请使用裸机部署或 host 网络模式（--network host）。' },
  { question: '从站扫描无结果？', answer: '确认从站已上电、网线连接正常，主站与从站在同一网段。可先使用 Wireshark 抓包确认 EtherCAT 帧。' },
  { question: 'PDO 周期不稳定？', answer: '评估 Go GC 对周期线程的影响，可调优 GOGC 参数或使用 runtime.LockOSThread 隔离周期线程。' },
  { question: '如何开发测试？', answer: '启动模拟模式（simulation: true），接口设为 lo，无需真实设备和网卡即可完成全部功能验证。' },
  { question: 'SDO 读写超时？', answer: 'SDO 通过 CoE 邮箱协议访问，响应较 PDO 慢。增大 timeout 参数（如 5000ms），确认对象字典 Index/SubIndex 正确。' },
  { question: '地址字节序？', answer: 'EtherCAT 默认大端字节序（#BE），可通过 #LE 后缀切换为小端。如 1:Tx:0#LE 表示小端读取。' },
]
</script>