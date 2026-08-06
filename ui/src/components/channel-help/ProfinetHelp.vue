<template>
  <article class="help-doc">
    <header class="help-doc__hero">
      <span class="protocol-tag protocol-tag--accent">Profinet IO</span>
      <p class="help-doc__lead">
        PROFINET IO 工业以太网协议，edgeCore 作为 IO 控制器通过 TCP（端口 34964）对 IO 设备进行非循环读写。
      </p>
    </header>
    <div class="help-doc__sections">
      <ChannelHelpBlock title="适用场景" text="西门子及兼容 PROFINET IO 的现场设备、远程 IO 模块数据采集与反控。" />
      <ChannelHelpBlock title="部署注意">
        <p class="help-doc__text">
          PROFINET IO 实时报文基于以太网帧传输，需将 edgeCore 部署在物理网关上并绑定真实网卡，不建议在 Docker 或虚拟机中使用。
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
          <li>创建通道，协议选择 Profinet IO，填写本地网口名称（如 eth0）。</li>
          <li>添加 IO 设备，填写设备名称、IP 地址、端口（默认 34964）及模块槽号/子槽号。</li>
          <li>设置采集间隔，进入点位页添加点位，地址格式为 SLOT:SUB_SLOT:INDEX。</li>
          <li>开发/测试时可启用模拟模式（simulation）无需真实设备。</li>
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
  { label: '本地网口', example: 'eth0', desc: '绑定用于 PROFINET 通信的物理网卡' },
  { label: '超时 / 重试', desc: 'TCP 连接与 RPC 读写超时、重试次数' },
  { label: '模拟模式', example: 'simulation: true', desc: '无真实设备时用于开发测试' },
]

const deviceParams = [
  { label: '设备名称', example: 'io-device-1', desc: 'IO 设备名称' },
  { label: '设备 IP', example: '192.168.1.20' },
  { label: '端口', example: '34964', desc: '默认 34964' },
  { label: '槽号 / 子槽号', example: '3 / 1', desc: 'IO 模块槽位' },
  { label: 'API / 标识', desc: '模块 API 列表与标识（可选）' },
  { label: '输入/输出长度', desc: '模组 IO 数据长度（字节）' },
]

const addressFormat = [
  { label: '格式', example: 'SLOT:SUB_SLOT:INDEX[.BIT][#ENDIAN]' },
  { label: 'SLOT', desc: '模组插入模块的槽号' },
  { label: 'SUB_SLOT', desc: '子槽号' },
  { label: 'INDEX', desc: '数据索引（字节偏移，从 0 开始）' },
  { label: 'BIT', desc: '可选，位偏移 0–7' },
  { label: 'ENDIAN', desc: '可选，#BE（默认）或 #LE' },
]

const addressExamples = [
  { label: 'int16', example: '3:1:0', desc: '槽 3 子槽 1 第 0,1 字节' },
  { label: 'uint16', example: '3:1:1', desc: '槽 3 子槽 1 第 1,2 字节' },
  { label: 'uint32', example: '3:2:3', desc: '槽 3 子槽 2 第 3–6 字节' },
  { label: 'float', example: '3:2:10', desc: '槽 3 子槽 2 第 10–13 字节' },
  { label: 'bit', example: '3:2:5.3', desc: '槽 3 子槽 2 第 5 字节第 3 位' },
]

const faqItems = [
  { question: '连接失败？', answer: '确认设备 IP 可达、端口 34964 开放，且 edgeCore 已绑定正确的物理网卡。' },
  { question: '读取值为 Bad？', answer: '检查槽号/子槽号/索引是否与 GSD 或设备手册一致，数据类型与字节长度匹配。' },
  { question: 'Docker 中无法通信？', answer: 'PROFINET IO 需直接访问物理网卡，请使用裸机或 host 网络模式部署。' },
]
</script>
