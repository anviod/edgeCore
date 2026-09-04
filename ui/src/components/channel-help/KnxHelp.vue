<template>
  <article class="help-doc">
    <header class="help-doc__hero">
      <span class="protocol-tag protocol-tag--accent">KNXnet/IP</span>
      <p class="help-doc__lead">
        KNX 楼宇自动化协议的 IP 隧道模式。edgeCore 作为隧道客户端，经 KNX IP 接口（网关）读写总线组地址（Group Address）。
      </p>
    </header>
    <div class="help-doc__sections">
      <ChannelHelpBlock title="适用场景" text="家居/楼宇 KNX 系统数据采集与反控，如照明、暖通、遮阳、场景联动等组地址点位。" />
      <ChannelHelpBlock title="基础连接">
        <ChannelHelpParamList :items="connectionParams" />
      </ChannelHelpBlock>
      <ChannelHelpBlock title="通信参数">
        <ChannelHelpParamList :items="commParams" />
      </ChannelHelpBlock>
      <ChannelHelpBlock title="组地址格式">
        <ChannelHelpParamList :items="addressExamples" />
      </ChannelHelpBlock>
      <ChannelHelpBlock title="数据类型">
        <ChannelHelpParamList :items="dataTypes" />
      </ChannelHelpBlock>
      <ChannelHelpBlock title="配置步骤">
        <ol class="help-doc-steps">
          <li>填写 KNX IP 网关地址（直连 IP，非组播）；或启用 discovery 自动发现网关。</li>
          <li>端口默认 3671；传输模式选 UDP（常见）或 TCP（部分接口要求）。</li>
          <li>在 ETS 中确认组地址与 DPT 类型，点位地址使用 main/middle/sub 格式。</li>
          <li>布尔/开关量用 BOOL；温度等浮点用 FLOAT；百分比/调光可用 UINT8。</li>
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

const connectionParams = [
  { label: '网关 IP (ip)', example: '192.168.1.50', desc: 'KNX IP 接口/网关的局域网地址' },
  { label: '端口', example: '3671', desc: 'KNXnet/IP 标准端口' },
  { label: '传输模式 (mode)', desc: 'UDP（默认）或 TCP' },
  { label: '自动发现 (discovery)', desc: 'true 时通过 SEARCH 发现网关；未填 ip 时自动选用首个响应设备' },
  { label: '发现超时', example: '3000', desc: 'discovery_timeout，单位 ms，默认 3000' },
  { label: '发现组播', example: '224.0.23.12:3671', desc: 'discovery_multicast，默认 KNX 标准组播地址' },
]

const commParams = [
  { label: '超时 / 重试', desc: 'timeout 建议 2000–5000 ms；max_retries 控制连接重试次数' },
  { label: '保活间隔', desc: 'heartbeat_interval 默认 60000 ms，发送 ConnectionState 维持隧道' },
  { label: '本地绑定 IP', desc: 'local_ip 可选，多网卡时指定出站网卡' },
]

const addressExamples = [
  { label: '三级组地址', example: '1/2/3' },
  { label: '二级组地址', example: '1/34' },
  { label: '带个体地址', example: '0/0/1,1.1.1' },
  { label: '子字节/位域', example: '0/0/1,1.1.1,2', desc: '第三位为读取的比特宽度（DPT B1/B2 等）' },
]

const dataTypes = [
  { label: 'BOOL / BIT', desc: '布尔，对应 DPT 1' },
  { label: 'INT8 / UINT8', desc: '8 位整数，如 DPT 5' },
  { label: 'INT16 / UINT16', desc: '16 位整数，如 DPT 7/8' },
  { label: 'INT32 / UINT32', desc: '32 位整数' },
  { label: 'FLOAT', desc: '浮点，2 字节 DPT 9 或 4 字节 IEEE' },
]

const faqItems = [
  { question: '连接失败或 Health Bad？', answer: '确认网关 IP 可达、3671 端口开放、ETS 中隧道已启用；尝试切换 UDP/TCP 模式。' },
  { question: 'discovery 找不到网关？', answer: '检查与 KNX 接口同一网段、防火墙允许 UDP 3671 组播；可手动填写 ip 跳过发现。' },
  { question: '读值为 0 或与 ETS 不一致？', answer: '核对组地址、DPT 与数据类型是否匹配；子字节点位需配置 BIT 宽度参数。' },
  { question: 'UDP 与 TCP 如何选择？', answer: '多数 Weinzierl、ABB 等接口默认 UDP 隧道；若厂商文档要求 TCP，将 mode 设为 TCP。' },
  { question: '写操作无响应？', answer: '确认点位 readwrite 为 RW，组地址在 ETS 中允许写入，且总线设备在线。' },
]
</script>
