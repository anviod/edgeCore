<template>
  <a-modal v-model:visible="visible" title="BACnet 服务端接入文档" :width="900" :footer="false" modal-class="northbound-help-modal" unmount-on-close>
    <a-tabs v-model:active-key="activeTab" type="line">
      <a-tab-pane key="connection" title="连接配置">
        <div class="nb-help-tab">
          <header class="nb-help-hero">
            <h4 class="nb-help-hero__title">连接配置 (Connection)</h4>
            <p class="nb-help-hero__lead">使用 BACnet 客户端（如 Yabe、BAC0、CAS BACnet Explorer）通过 BACnet/IP 协议连接到本网关。</p>
          </header>

          <div class="nb-help-topic-card nb-help-topic-card--primary">
            <div class="nb-help-topic-card__body">
              <div class="nb-help-topic-card__label">服务参数</div>
              <a-descriptions :column="1" size="small" bordered style="margin-bottom: 12px">
                <a-descriptions-item label="端口">{{ port }}</a-descriptions-item>
                <a-descriptions-item label="设备实例 ID">{{ deviceId || '自动生成' }}</a-descriptions-item>
                <a-descriptions-item label="设备名称">{{ deviceName }}</a-descriptions-item>
              </a-descriptions>
              <p class="nb-help-hint">提示：BACnet 标准端口 47808 (0xBAC0)，与南向 BACnet 驱动端口 47809 分离避免冲突。</p>
            </div>
          </div>

          <div class="nb-help-block">
            <div class="nb-help-block-title">设备发现机制</div>
            <a-alert type="success" class="nb-help-alert">
              <div><strong>设备发现流程：</strong></div>
              <ol class="nb-help-list">
                <li>BACnet 主站发送 <strong>Who-Is</strong> 广播消息到网络</li>
                <li>网关自动回复 <strong>I-Am</strong> 消息，包含设备实例 ID 和 IP 地址</li>
                <li>主站即可在设备列表中找到本网关，无需手动输入 IP</li>
              </ol>
            </a-alert>
          </div>

          <div class="nb-help-block">
            <div class="nb-help-block-title">支持的 BACnet 服务</div>
            <a-table :columns="serviceColumns" :data="serviceData" size="small" :bordered="{ cell: true }" :pagination="false" />
          </div>

          <div class="nb-help-block">
            <div class="nb-help-block-title">客户端指南</div>
            <a-collapse class="help-doc-faq" :default-active-key="['yabe']" :bordered="false" expand-icon-position="right">
              <a-collapse-item header="Yabe（Yet Another BACnet Explorer）推荐" key="yabe">
                <p>开源跨平台 BACnet 客户端，支持设备发现、属性浏览和值写入。</p>
                <p>
                  <a href="https://sourceforge.net/projects/yetanotherbacnetexplorer/" target="_blank" class="nb-help-link">下载地址 (Download)</a>
                </p>
                <div class="nb-help-steps-box">
                  <strong>连接步骤：</strong>
                  <ol>
                    <li>确保客户端与网关在同一网络或 VLAN 中。</li>
                    <li>打开 Yabe，它会自动发送 Who-Is 广播发现设备。</li>
                    <li>在左侧设备树中找到设备名称为 <strong>{{ deviceName }}</strong> 的设备。</li>
                    <li>展开设备节点，浏览所有 AnalogInput / BinaryInput 等对象。</li>
                    <li>双击可写对象（AnalogValue / BinaryValue）的 PresentValue 可进行写入操作。</li>
                  </ol>
                </div>
              </a-collapse-item>
              <a-collapse-item header="BAC0（Python 库）" key="bac0">
                <p>Python 下的 BACnet 客户端库，支持脚本化读写。</p>
                <div class="nb-help-steps-box">
                  <strong>使用示例：</strong>
                  <pre class="nb-help-pre">pip install BAC0
import BAC0
bacnet = BAC0.lite(ip='192.168.1.100')
# 读取点位
value = bacnet.read('1000 analogInput 1 presentValue')
# 写入点位
bacnet.write('1000 analogValue 3 presentValue', 25.5)</pre>
                </div>
              </a-collapse-item>
              <a-collapse-item header="BACnet 协议栈 (bacnet-stack)" key="bacnet-stack">
                <p>C 语言开源 BACnet 协议栈，适用于嵌入式场景。</p>
                <div class="nb-help-steps-box">
                  <strong>连接步骤：</strong>
                  <ol>
                    <li>编译 bacnet-stack 的 whois / readprop / writeprop 命令行工具。</li>
                    <li>发送 Who-Is：<code>./bin/bacwhois</code></li>
                    <li>读取属性：<code>./bin/bacrp {{ deviceId }} 0 1</code></li>
                    <li>写入属性：<code>./bin/bacwp {{ deviceId }} 2 3 85 4 25.5</code></li>
                  </ol>
                </div>
              </a-collapse-item>
            </a-collapse>
          </div>
        </div>
      </a-tab-pane>

      <a-tab-pane key="objects" title="对象映射">
        <div class="nb-help-tab">
          <header class="nb-help-hero">
            <h4 class="nb-help-hero__title">对象映射 (Object Mapping)</h4>
            <p class="nb-help-hero__lead">edgeCore 南向设备的点位会根据其 DataType 和 ReadWrite 属性自动映射为对应的 BACnet 标准对象类型。</p>
          </header>

          <div class="nb-help-block">
            <div class="nb-help-block-title">点位类型映射规则</div>
            <a-table :columns="mappingColumns" :data="mappingData" size="small" :bordered="{ cell: true }" :pagination="false" />
          </div>

          <div class="nb-help-block">
            <div class="nb-help-block-title">对象实例号</div>
            <p class="nb-help-hero__lead">每个点位按顺序分配唯一的 BACnet 对象实例号（从 1 开始递增）。实例号在 BACnet 设备内唯一，用于区分不同的数据点。</p>
          </div>

          <div class="nb-help-block">
            <div class="nb-help-block-title">对象属性</div>
            <a-table :columns="propColumns" :data="propData" size="small" :bordered="{ cell: true }" :pagination="false" />
          </div>
        </div>
      </a-tab-pane>

      <a-tab-pane key="ref" title="参考">
        <div class="nb-help-tab">
          <header class="nb-help-hero">
            <h4 class="nb-help-hero__title">参考信息 (Reference)</h4>
            <p class="nb-help-hero__lead">BACnet 协议标准与相关资源。</p>
          </header>

          <div class="nb-help-block">
            <div class="nb-help-block-title">协议标准</div>
            <ul class="nb-help-list">
              <li>ISO 16484-5: Building Automation and Control Networks — Data Communication Protocol</li>
              <li>ANSI/ASHRAE Standard 135: BACnet — A Data Communication Protocol for Building Automation and Control Networks</li>
            </ul>
          </div>

          <div class="nb-help-block">
            <div class="nb-help-block-title">设备标识参数</div>
            <a-table :columns="idColumns" :data="idData" size="small" :bordered="{ cell: true }" :pagination="false" />
          </div>

          <div class="nb-help-block">
            <div class="nb-help-block-title">常见问题</div>
            <ul class="nb-help-list nb-help-list--tertiary">
              <li>如果主站无法发现设备，请确认主站与网关在同一子网（Who-Is 广播不跨子网）。</li>
              <li>如果读取值为空或错误，请检查南向设备是否在线且点位数据是否正常采集。</li>
              <li>如果写入失败，请确认点位在 edgeCore 中配置为 RW（可读写），且设备支持写入操作。</li>
              <li>BACnet 对象实例号在设备内唯一，不同设备之间可以重复使用相同的实例号。</li>
            </ul>
          </div>
        </div>
      </a-tab-pane>
    </a-tabs>
  </a-modal>
</template>

<script setup>
import { ref, computed } from 'vue'

const props = defineProps({
  visible: { type: Boolean, default: false },
  port: { type: Number, default: 47808 },
  deviceId: { type: Number, default: 0 },
  deviceName: { type: String, default: 'edgeCore-Gateway' }
})

const emit = defineEmits(['update:visible'])

const visible = computed({
  get: () => props.visible,
  set: (val) => emit('update:visible', val)
})
const activeTab = ref('connection')

const serviceColumns = [
  { title: '服务', dataIndex: 'service', width: 200 },
  { title: '类型', dataIndex: 'type', width: 100 },
  { title: '说明', dataIndex: 'desc' }
]

const serviceData = [
  { service: 'Who-Is / I-Am', type: '无确认', desc: '设备发现：主站广播 Who-Is，Server 回复 I-Am' },
  { service: 'ReadProperty', type: '确认', desc: '读取单个对象属性值（含 PresentValue）' },
  { service: 'WriteProperty', type: '确认', desc: '写入单个对象属性值，会转发到南向设备' },
  { service: 'ReadPropertyMultiple', type: '确认', desc: '批量读取多个对象属性' },
  { service: 'WritePropertyMultiple', type: '确认', desc: '批量写入多个对象属性' },
  { service: 'SubscribeCOV', type: '确认', desc: '订阅 COV（Change of Value）通知' },
  { service: 'COVNotification', type: '确认/无确认', desc: '点位值变化时自动推送通知' }
]

const mappingColumns = [
  { title: 'edgeCore DataType', dataIndex: 'dataType', width: 160 },
  { title: 'ReadWrite', dataIndex: 'rw', width: 80, align: 'center' },
  { title: 'BACnet 对象类型', dataIndex: 'bacnetType', width: 160 },
  { title: '类型编号', dataIndex: 'typeId', width: 80, align: 'center' },
  { title: 'PresentValue 类型', dataIndex: 'valueType', width: 120 }
]

const mappingData = [
  { dataType: 'float32 / float64 / int', rw: 'R', bacnetType: 'AnalogInput', typeId: '0', valueType: 'REAL' },
  { dataType: 'float32 / float64 / int', rw: 'RW', bacnetType: 'AnalogValue', typeId: '2', valueType: 'REAL' },
  { dataType: 'bool / boolean', rw: 'R', bacnetType: 'BinaryInput', typeId: '3', valueType: 'BOOL' },
  { dataType: 'bool / boolean', rw: 'RW', bacnetType: 'BinaryValue', typeId: '5', valueType: 'BOOL' },
  { dataType: 'string', rw: 'R', bacnetType: 'MultiStateInput', typeId: '13', valueType: 'UINT32' },
  { dataType: 'string', rw: 'RW', bacnetType: 'MultiStateValue', typeId: '19', valueType: 'UINT32' }
]

const propColumns = [
  { title: '属性', dataIndex: 'prop', width: 160 },
  { title: 'BACnet 属性 ID', dataIndex: 'id', width: 120 },
  { title: '说明', dataIndex: 'desc' }
]

const propData = [
  { prop: 'PresentValue', id: '85', desc: '当前值，从南向设备实时数据同步' },
  { prop: 'ObjectName', id: '77', desc: '对象名称，对应 edgeCore 点位名称' },
  { prop: 'Description', id: '28', desc: '对象描述，包含通道/设备/点位完整路径' }
]

const idColumns = [
  { title: '参数', dataIndex: 'param', width: 160 },
  { title: '说明', dataIndex: 'desc' },
  { title: '默认值', dataIndex: 'def', width: 160 }
]

const idData = [
  { param: '设备实例 ID', desc: 'BACnet 网络中唯一标识此设备的编号 (0-4194303)', def: '自动生成 (FNV-32a)' },
  { param: '设备名称', desc: '设备的人类可读名称', def: 'edgeCore-Gateway' },
  { param: '厂商 ID', desc: 'BACnet 厂商识别码', def: '999' },
  { param: '最大 PDU', desc: '最大协议数据单元大小（字节）', def: '1476' }
]
</script>