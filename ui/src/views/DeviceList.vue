<template>
  <div class="page-shell device-list-container list-detail-page">
    <div class="page-header device-header list-page-header">
      <div class="header-left">
        <a-button type="outline" size="small" class="list-back-btn" @click="router.push('/channels')">
          <template #icon><IconArrowLeft /></template>
          返回通道
        </a-button>
        <div class="header-info header-info--inline">
          <span class="protocol-tag protocol-tag--accent">{{ formatProtocolTag(channelProtocol) }}</span>
          <h2 class="page-title title-text">设备列表</h2>
        </div>
      </div>

      <div class="header-right header-actions">
        <a-space size="small" wrap>
          <a-button type="outline" size="small" @click="router.push('/virtual-shadows')">
            <template #icon><IconThunderbolt /></template>
            虚拟影子
          </a-button>
          <a-button v-if="selected.length > 0" status="danger" type="outline" size="small" @click="confirmBatchDelete">
            <template #icon><IconDelete /></template>
            批量删除 ({{ selected.length }})
          </a-button>
          <a-button v-if="channelProtocol === 'bacnet-ip' || channelProtocol === 'opc-ua'" type="outline" status="success" size="small" @click="openScanDialog()">
            <template #icon><IconScan /></template>
            扫描设备
          </a-button>
          <a-button v-if="channelProtocol && channelProtocol.includes('modbus')" type="outline" status="success" size="small" @click="openBatchModbusDialog">
            <template #icon><IconPlus /></template>
            批量新增从站
          </a-button>
          <a-button type="primary" size="small" @click="openDialog()">
            <template #icon><IconPlus /></template>
            新增设备
          </a-button>
        </a-space>
      </div>
    </div>

    <div class="list-detail-body">
      <a-spin :loading="loading" class="list-detail-spin">
        <div class="table-container saas-table">
          <a-table
            class="device-table"
            :columns="tableColumns"
            :data="devices"
            :loading="loading"
            :row-selection="rowSelection"
            v-model:selected-keys="selected"
            row-key="id"
            size="small"
            :pagination="{ showTotal: true, showPageSize: true }"
          >
            <template #enable="{ record }">
              <span class="table-cell-semantic">
                <a-switch
                  v-model="record.enable"
                  size="small"
                  @change="toggleDeviceStatus(record)"
                  :loading="record.statusLoading"
                />
              </span>
            </template>

            <template #name="{ record }">
              <div class="device-name-cell">
                <span class="main-name">{{ record.name }}</span>
                <span class="sub-id">ID: {{ record.id }}</span>
              </div>
            </template>

            <template #location="{ record }">
              <div class="device-location-cell">
                <span v-if="record.station_name || record.station_code" class="loc-line">
                  {{ record.station_name || '-' }}
                  <span v-if="record.station_code" class="loc-code">{{ record.station_code }}</span>
                </span>
                <span v-if="record.room_name || record.room_code" class="loc-line loc-sub">
                  {{ record.room_name || '-' }}
                  <span v-if="record.room_code" class="loc-code">{{ record.room_code }}</span>
                </span>
                <span v-if="!record.station_name && !record.station_code && !record.room_name && !record.room_code" class="loc-empty">-</span>
              </div>
            </template>

            <template #interval="{ record }">
              <span class="table-cell-semantic">
                <a-tag size="small" bordered>
                  <IconClockCircle :size="12" style="margin-right: 4px" />
                  {{ record.interval }}
                </a-tag>
              </span>
            </template>

            <template #state="{ record }">
              <span class="table-cell-semantic">
                <a-tag :color="getDeviceStateColor(record.state)" size="small">
                  {{ getDeviceStateText(record.state) }}
                </a-tag>
              </span>
            </template>

            <template #actions="{ record }">
              <div class="table-ops">
                <a-tooltip content="查看点位">
                  <a-button type="text" size="mini" @click="goToPoints(record)">
                    <IconEye :size="14" />
                  </a-button>
                </a-tooltip>
                <a-tooltip content="规则链">
                  <a-button type="text" size="mini" @click="showRuleUsage(record)">
                    <IconLink :size="14" />
                  </a-button>
                </a-tooltip>
                <a-tooltip content="历史数据">
                  <a-button type="text" size="mini" @click="openHistoryDialog(record)">
                    <IconClockCircle :size="14" />
                  </a-button>
                </a-tooltip>
                <a-divider direction="vertical" />
                <a-button type="text" size="mini" @click="openDialog(record)">编辑</a-button>
                <a-button type="text" size="mini" status="danger" @click="confirmDelete(record)">删除</a-button>
              </div>
            </template>
          </a-table>
        </div>
      </a-spin>

      <div class="list-detail-meta">
        <span class="list-detail-meta__dot" :class="listDetailMetaDotClass"></span>
        <span class="list-detail-meta__text">{{ listDetailMetaText }}</span>
      </div>
    </div>

    <a-modal
      v-model:visible="dialog"
      :title="form.id && isEdit ? '编辑设备' : '新增设备'"
      :width="960"
      modal-class="device-config-modal channel-config-modal"
      :ok-loading="saving"
      @before-ok="saveDevice"
      @cancel="closeDialog"
    >
      <a-form :model="form" layout="vertical" class="device-config-form channel-config-form flow-form form-controls-md">
        <div class="device-form-section">
          <div class="device-section-title">
            <span class="device-section-title__bar"></span>
            <span class="device-section-title__text">基本信息</span>
            <span class="device-section-title__line"></span>
          </div>
        <a-row :gutter="16">
          <a-col :span="6">
            <a-form-item field="id" label="设备ID" required>
              <a-input v-model="form.id" placeholder="设备唯一标识" :disabled="isEdit" />
            </a-form-item>
          </a-col>
          <a-col :span="6">
            <a-form-item field="name" label="设备名称" required>
              <a-input v-model="form.name" placeholder="例如: 智能电表_01" />
            </a-form-item>
          </a-col>
          <a-col :span="6">
            <a-form-item field="interval" label="采集间隔" required>
              <a-input v-model="form.interval" placeholder="例如: 5s, 1m" />
            </a-form-item>
          </a-col>
          <a-col :span="6">
            <a-form-item field="enable" label="启用状态">
              <a-switch v-model="form.enable" />
            </a-form-item>
          </a-col>
        </a-row>

        </div>

        <div class="device-form-section">
        <div class="device-section-title">
          <span class="device-section-title__bar"></span>
          <span class="device-section-title__text">空间属性</span>
          <span class="device-section-title__line"></span>
        </div>

        <a-row :gutter="16">
          <a-col :span="6">
            <a-form-item field="stationName" label="局站名称">
              <a-input v-model="form.stationName" placeholder="例如: 一体化冷站" />
            </a-form-item>
          </a-col>
          <a-col :span="6">
            <a-form-item field="stationCode" label="局站编码">
              <a-input v-model="form.stationCode" placeholder="例如: ST01.Z001" />
            </a-form-item>
          </a-col>
          <a-col :span="6">
            <a-form-item field="roomName" label="机房名称">
              <a-input v-model="form.roomName" placeholder="例如: 动力机房/1楼/1号电力室" />
            </a-form-item>
          </a-col>
          <a-col :span="6">
            <a-form-item field="roomCode" label="机房编码">
              <a-input v-model="form.roomCode" placeholder="例如: ST01.R001" />
            </a-form-item>
          </a-col>
        </a-row>
        
        </div>

        <div class="device-form-section">
        <div class="device-section-title">
          <span class="device-section-title__bar"></span>
          <span class="device-section-title__text">通信配置</span>
          <span class="device-section-title__line"></span>
        </div>
        
        <template v-if="channelProtocol === 'dlt645'">
          <a-row :gutter="16">
            <a-col :span="12">
              <a-form-item field="dlt645Address" label="设备地址" required>
                <a-input v-model="form.dlt645Address" placeholder="210220003011" />
              </a-form-item>
            </a-col>
            <a-col :span="12">
              <a-form-item field="dlt645AutoPointsEnabled" label="导入标准点位">
                <a-switch v-model="form.dlt645AutoPointsEnabled" />
                <template #extra>按 DL/T 645-2007 标准 DI 模板自动生成常用采集点位</template>
              </a-form-item>
            </a-col>
          </a-row>
        </template>

        <template v-if="channelProtocol === 'knxnet-ip'">
          <a-form-item field="knxIndividualAddress" label="默认个体地址 (可选)">
            <a-input v-model="form.knxIndividualAddress" placeholder="1.1.1" />
            <template #extra>
              点位地址支持 `1/2/3` 或 `1/2/3,1.1.1,2`；这里可预设设备级个体地址用于统一管理。
            </template>
          </a-form-item>
        </template>

        <template v-if="channelProtocol === 'profinet-io'">
          <a-row :gutter="16">
            <a-col :span="8">
              <a-form-item field="pnioDeviceName" label="设备名称" required>
                <a-input v-model="form.pnioDeviceName" placeholder="io-device-1" />
              </a-form-item>
            </a-col>
            <a-col :span="8">
              <a-form-item field="pnioIp" label="设备 IP 地址" required>
                <a-input v-model="form.pnioIp" placeholder="192.168.1.20" />
              </a-form-item>
            </a-col>
            <a-col :span="8">
              <a-form-item field="pnioPort" label="设备端口">
                <a-input-number v-model="form.pnioPort" :min="1" :max="65535" placeholder="34964" />
              </a-form-item>
            </a-col>
          </a-row>
          <a-row :gutter="16">
            <a-col :span="8">
              <a-form-item field="pnioSlot" label="槽号">
                <a-input-number v-model="form.pnioSlot" :min="0" :max="65535" placeholder="3" />
              </a-form-item>
            </a-col>
            <a-col :span="8">
              <a-form-item field="pnioSubslot" label="子槽号">
                <a-input-number v-model="form.pnioSubslot" :min="0" :max="65535" placeholder="1" />
              </a-form-item>
            </a-col>
            <a-col :span="8">
              <a-form-item field="pnioApi" label="API 列表">
                <a-input v-model="form.pnioApi" placeholder="0" />
              </a-form-item>
            </a-col>
          </a-row>
          <a-row :gutter="16">
            <a-col :span="8">
              <a-form-item field="pnioIdent" label="标识">
                <a-input v-model="form.pnioIdent" placeholder="" />
              </a-form-item>
            </a-col>
            <a-col :span="8">
              <a-form-item field="pnioSubIdent" label="子标识">
                <a-input v-model="form.pnioSubIdent" placeholder="" />
              </a-form-item>
            </a-col>
            <a-col :span="8">
              <a-form-item field="pnioProperties" label="属性">
                <a-input v-model="form.pnioProperties" placeholder="" />
              </a-form-item>
            </a-col>
          </a-row>
          <a-row :gutter="16">
            <a-col :span="12">
              <a-form-item field="pnioInputLength" label="输入数据长度">
                <a-input-number v-model="form.pnioInputLength" :min="0" :max="65535" placeholder="64" />
              </a-form-item>
            </a-col>
            <a-col :span="12">
              <a-form-item field="pnioOutputLength" label="输出数据长度">
                <a-input-number v-model="form.pnioOutputLength" :min="0" :max="65535" placeholder="32" />
              </a-form-item>
            </a-col>
          </a-row>
        </template>

        <template v-if="channelProtocol === 'ethercat'">
          <a-form-item field="ecatPosition" label="从站位置" required>
            <a-input-number v-model="form.ecatPosition" :min="1" :max="65535" placeholder="1" />
            <template #extra>
              EtherCAT 总线上的从站物理位置（1-based positional addressing）
            </template>
          </a-form-item>
          <a-row :gutter="16">
            <a-col :span="12">
              <a-form-item field="ecatVendorId" label="厂商 ID">
                <a-input v-model="form.ecatVendorId" placeholder="0x00000002" />
              </a-form-item>
            </a-col>
            <a-col :span="12">
              <a-form-item field="ecatProductCode" label="产品代码">
                <a-input v-model="form.ecatProductCode" placeholder="0x07D43052" />
              </a-form-item>
            </a-col>
          </a-row>
          <a-row :gutter="16">
            <a-col :span="8">
              <a-form-item field="ecatTxPdoSize" label="TxPDO 大小 (字节)">
                <a-input-number v-model="form.ecatTxPdoSize" :min="0" :max="65535" placeholder="16" />
              </a-form-item>
            </a-col>
            <a-col :span="8">
              <a-form-item field="ecatRxPdoSize" label="RxPDO 大小 (字节)">
                <a-input-number v-model="form.ecatRxPdoSize" :min="0" :max="65535" placeholder="8" />
              </a-form-item>
            </a-col>
            <a-col :span="8">
              <a-form-item field="ecatAlias" label="别名地址">
                <a-input-number v-model="form.ecatAlias" :min="0" :max="65535" placeholder="0" />
              </a-form-item>
            </a-col>
          </a-row>
          <a-form-item field="ecatRunMode" label="运行模式">
            <a-select v-model="form.ecatRunMode" placeholder="pdo">
              <a-option value="pdo">PDO (过程数据)</a-option>
              <a-option value="sdo">SDO (邮箱数据)</a-option>
            </a-select>
            <template #extra>
              PDO 模式用于高速周期性数据交换；SDO 模式用于参数配置和诊断
            </template>
          </a-form-item>
        </template>
        
        <template v-if="channelProtocol && channelProtocol.includes('modbus')">
          <a-row :gutter="16">
            <a-col :span="12">
              <a-form-item field="modbusSlaveId" label="从机ID" required>
                <a-input-number v-model="form.modbusSlaveId" :min="1" placeholder="1" />
              </a-form-item>
            </a-col>
            <a-col :span="12">
              <a-form-item field="startAddressMode" label="地址模式">
                <a-radio-group v-model="form.startAddressMode">
                  <a-radio :value="0">0-based</a-radio>
                  <a-radio :value="1">1-based</a-radio>
                </a-radio-group>
              </a-form-item>
            </a-col>
          </a-row>

          <div class="device-subsection-title">
            <span class="device-subsection-title__bar"></span>
            <span>寄存器区块（批量创建点位）</span>
          </div>
          <a-form-item field="autoPointsEnabled" label="启用区块">
            <a-switch v-model="form.autoPointsEnabled" />
            <template #extra>创建设备时按区间自动生成保持寄存器点位（功能码 0x03）</template>
          </a-form-item>
          <template v-if="form.autoPointsEnabled">
            <a-row :gutter="16">
              <a-col :span="8">
                <a-form-item field="autoPointsStart" label="起始地址">
                  <a-input-number v-model="form.autoPointsStart" :min="0" :max="65535" />
                </a-form-item>
              </a-col>
              <a-col :span="8">
                <a-form-item field="autoPointsEnd" label="结束地址">
                  <a-input-number v-model="form.autoPointsEnd" :min="0" :max="65535" />
                </a-form-item>
              </a-col>
              <a-col :span="8">
                <a-form-item field="autoPointsCount" label="点位数量">
                  <a-input :model-value="autoPointsCountLabel" disabled />
                </a-form-item>
              </a-col>
            </a-row>
            <a-row :gutter="16">
              <a-col :span="8">
                <a-form-item field="autoPointsDatatype" label="数据类型">
                  <a-select
                    v-model="form.autoPointsDatatype" :options="[
                      { label: 'int16', value: 'int16' },
                      { label: 'uint16', value: 'uint16' },
                      { label: 'int32', value: 'int32' },
                      { label: 'float32', value: 'float32' }
                    ]"
                  />
                </a-form-item>
              </a-col>
              <a-col :span="8">
                <a-form-item field="autoPointsReadWrite" label="读写">
                  <a-select
                    v-model="form.autoPointsReadWrite" :options="[
                      { label: '只读 (R)', value: 'R' },
                      { label: '读写 (RW)', value: 'RW' }
                    ]"
                  />
                </a-form-item>
              </a-col>
              <a-col :span="8">
                <a-form-item field="autoPointsRegisterType" label="寄存器类型">
                  <a-select
                    v-model="form.autoPointsRegisterType" :options="[
                      { label: '保持寄存器 (0x03)', value: 'holding' }
                    ]"
                  />
                </a-form-item>
              </a-col>
            </a-row>
            <a-form-item v-if="isEdit" field="regeneratePoints" label="重新生成">
              <a-switch v-model="form.regeneratePoints" />
              <template #extra>开启后保存时将按新区间重新生成点位（保留同 ID 现有点配置）</template>
            </a-form-item>
          </template>
        </template>
        
        <template v-if="channelProtocol === 'bacnet-ip'">
          <a-row :gutter="16">
            <a-col :span="8">
              <a-form-item field="bacnet_device_id" label="BACnet设备ID" required>
                <a-input-number v-model="form.bacnet_device_id" placeholder="1001" />
              </a-form-item>
            </a-col>
            <a-col :span="8">
              <a-form-item field="ip" label="IP地址">
                <a-input v-model="form.ip" placeholder="192.168.1.100" />
              </a-form-item>
            </a-col>
            <a-col :span="8">
              <a-form-item field="port" label="端口">
                <a-input-number v-model="form.port" placeholder="47808" />
              </a-form-item>
            </a-col>
          </a-row>
        </template>
        
        <template v-if="channelProtocol === 'opc-ua'">
          <a-form-item field="endpoint" label="Endpoint URL">
            <a-input
              v-model="form.config.endpoint"
              :placeholder="channelOpcUaEndpoint || 'opc.tcp://192.168.1.10:4840'"
            />
            <template v-if="channelOpcUaEndpoint" #extra>
              留空时将自动使用通道 Endpoint：{{ channelOpcUaEndpoint }}
            </template>
          </a-form-item>
          <a-row :gutter="16">
            <a-col :span="8">
              <a-form-item field="security_policy" label="安全策略">
                <a-select
                  v-model="form.config.security_policy" :options="[
                    { label: 'None', value: 'None' },
                    { label: 'Basic128Rsa15', value: 'Basic128Rsa15' },
                    { label: 'Basic256', value: 'Basic256' },
                    { label: 'Basic256Sha256', value: 'Basic256Sha256' }
                  ]"
                />
              </a-form-item>
            </a-col>
            <a-col :span="8">
              <a-form-item field="security_mode" label="安全模式">
                <a-select
                  v-model="form.config.security_mode" :options="[
                    { label: 'None', value: 'None' },
                    { label: 'Sign', value: 'Sign' },
                    { label: 'SignAndEncrypt', value: 'SignAndEncrypt' }
                  ]"
                />
              </a-form-item>
            </a-col>
            <a-col :span="8">
              <a-form-item field="auth_method" label="认证方式">
                <a-select
                  v-model="form.config.auth_method" :options="[
                    { label: 'Anonymous', value: 'Anonymous' },
                    { label: 'UserName', value: 'UserName' },
                    { label: 'Certificate', value: 'Certificate' }
                  ]"
                />
              </a-form-item>
            </a-col>
          </a-row>
          
          <template v-if="form.config.auth_method === 'UserName'">
            <a-row :gutter="16">
              <a-col :span="12">
                <a-form-item field="username" label="用户名">
                  <a-input v-model="form.config.username" />
                </a-form-item>
              </a-col>
              <a-col :span="12">
                <a-form-item field="password" label="密码">
                  <a-input-password v-model="form.config.password" />
                </a-form-item>
              </a-col>
            </a-row>
          </template>
          
          <template v-if="form.config.auth_method === 'Certificate'">
            <a-form-item field="certificate_file" label="证书路径">
              <a-input v-model="form.config.certificate_file" />
            </a-form-item>
            <a-form-item field="private_key_file" label="私钥路径">
              <a-input v-model="form.config.private_key_file" />
            </a-form-item>
          </template>
        </template>
        
        </div>

        <div class="device-form-section">
        <div class="device-section-title">
          <span class="device-section-title__bar"></span>
          <span class="device-section-title__text">历史数据存储</span>
          <span class="device-section-title__line"></span>
          <a-switch v-model="form.storageEnable" />
        </div>
        
        <template v-if="form.storageEnable">
          <a-row :gutter="16">
            <a-col :span="8">
              <a-form-item field="storageStrategy" label="存储策略">
                <a-select
                  v-model="form.storageStrategy" :options="[
                    { label: '实时 (每次更新)', value: 'realtime' },
                    { label: '定时间隔 (全量快照)', value: 'interval' }
                  ]"
                />
              </a-form-item>
            </a-col>
            <a-col :span="8" v-if="form.storageStrategy === 'interval'">
              <a-form-item field="storageInterval" label="存储间隔(分钟)">
                <a-input-number v-model="form.storageInterval" :min="1" placeholder="1" />
              </a-form-item>
            </a-col>
            <a-col :span="8">
              <a-form-item field="storageMaxRecords" label="最大记录数">
                <a-input-number v-model="form.storageMaxRecords" :min="1" placeholder="1000" />
              </a-form-item>
            </a-col>
          </a-row>
        </template>
        
        </div>

        <div class="device-form-section">
        <div class="device-section-title">
          <span class="device-section-title__bar"></span>
          <span class="device-section-title__text">高级配置</span>
          <span class="device-section-title__line"></span>
        </div>
        
        <a-collapse :bordered="false" expand-icon-position="right" class="device-json-collapse">
          <a-collapse-item key="json" header="JSON 配置（可选，仅高级场景使用）">
            <a-form-item field="configStr">
              <a-textarea v-model="form.configStr" placeholder="{&quot;key&quot;: &quot;value&quot;}" :auto-size="{ minRows: 3, maxRows: 8 }" />
            </a-form-item>
          </a-collapse-item>
        </a-collapse>
        </div>
      </a-form>
    </a-modal>

    <a-modal
      v-model:visible="batchModbusDialog"
      title="批量新增 Modbus 从站"
      :width="760"
      modal-class="channel-config-modal channel-config-modal--batch"
      :ok-loading="batchModbusLoading"
      ok-text="批量创建"
      @ok="submitBatchModbus"
      @cancel="batchModbusDialog = false"
    >
      <p class="modal-intro modal-intro--compact">
        一次创建多台从站设备，每台按寄存器区间自动生成点位。
      </p>
      <a-form :model="batchModbusForm" layout="vertical" class="channel-config-form batch-modbus-form form-controls-md">
        <div class="batch-form-fields">
          <div class="form-field">
            <div class="field-label">从站 ID</div>
            <div class="batch-range-control">
              <a-input-number
                v-model="batchModbusForm.slaveStart"
                :min="1"
                :max="247"
                placeholder="1"
              />
              <span class="batch-range-sep">至</span>
              <a-input-number
                v-model="batchModbusForm.slaveEnd"
                :min="1"
                :max="247"
                placeholder="7"
              />
            </div>
          </div>

          <div class="batch-form-row">
            <div class="form-field">
              <div class="field-label">寄存器</div>
              <div class="batch-range-control">
                <a-input-number
                  v-model="batchModbusForm.regStart"
                  :min="0"
                  :max="65535"
                  placeholder="0"
                />
                <span class="batch-range-sep">至</span>
                <a-input-number
                  v-model="batchModbusForm.regEnd"
                  :min="0"
                  :max="65535"
                  placeholder="199"
                />
              </div>
            </div>
            <div class="form-field">
              <div class="field-label">寄存器类型</div>
              <a-select
                v-model="batchModbusForm.register_type"
                :options="batchModbusRegisterTypes"
                @change="onBatchRegisterTypeChange"
              />
            </div>
          </div>

          <div class="batch-form-row">
            <div class="form-field">
              <div class="field-label">采集间隔</div>
              <a-input v-model="batchModbusForm.interval" placeholder="1s" />
            </div>
            <div class="form-field">
              <div class="field-label">数据类型</div>
              <a-select
                v-model="batchModbusForm.datatype"
                :options="[
                  { label: 'int16', value: 'int16' },
                  { label: 'uint16', value: 'uint16' }
                ]"
                placeholder="选择数据类型"
              />
            </div>
          </div>

          <div class="batch-preview-block">
            <div class="field-label batch-preview-block__label">创建预览</div>
            <div class="batch-preview-stats">
              <div class="batch-preview-stat">
                <span class="batch-preview-stat__value">{{ batchModbusPreview.slaves }}</span>
                <span class="batch-preview-stat__label">设备</span>
              </div>
              <div class="batch-preview-stat">
                <span class="batch-preview-stat__value">{{ batchModbusPreview.points }}</span>
                <span class="batch-preview-stat__label">点位 / 台</span>
              </div>
              <div class="batch-preview-stat batch-preview-stat--accent">
                <span class="batch-preview-stat__value">{{ batchModbusPreview.total }}</span>
                <span class="batch-preview-stat__label">点位合计</span>
              </div>
            </div>
          </div>
        </div>
      </a-form>
    </a-modal>

    <HistoryModal v-model:visible="historyModalVisible" :device="historyDevice" />

    <a-modal v-model:visible="deleteDialog" title="确认删除" :ok-loading="deleting" @ok="executeDelete" @cancel="deleteDialog = false">
      <p>{{ itemToDelete ? '确定要删除该设备吗？' : `确定要删除选中的 ${selected.length} 个设备吗？` }}此操作无法撤销。</p>
    </a-modal>

    <a-modal v-model:visible="ruleUsageDialog.show" title="关联规则" width="80%" @cancel="ruleUsageDialog.show = false">
      <a-list v-if="ruleUsageDialog.rules.length > 0" :data="ruleUsageDialog.rules">
        <template #item="{ item }">
          <a-list-item>
            <a-list-item-meta :title="item.name" :description="item.id">
              <template #avatar>
                <icon-link :size="24" />
              </template>
            </a-list-item-meta>
            <template #actions>
              <a-button type="text" @click="goToRule(item.id)">查看配置</a-button>
            </template>
          </a-list-item>
        </template>
      </a-list>
      <a-empty v-else description="该设备未被任何规则引用" />
    </a-modal>

    <a-modal v-model:visible="scanDialog" title="扫描设备" width="900px" @cancel="scanDialog = false" :mask-closable="false">
      <template #footer>
        <a-space>
          <a-button @click="scanDialog = false">取消</a-button>
          <a-button type="primary" :loading="isAddingDevices" :disabled="selectedScanDevices.length === 0" @click="addSelectedDevices">
            添加选定设备 ({{ selectedScanDevices.length }})
          </a-button>
        </a-space>
      </template>
      
      <a-space direction="vertical" :size="16" fill>
        <a-row :gutter="[24, 16]" align="center">
          <a-col :span="12">
            <a-space>
              <a-button type="outline" status="primary" :loading="isScanning" :disabled="isScanning" @click="scanDevices">
                <template #icon>
                  <icon-scan />
                </template>
                开始扫描
              </a-button>
              <a-text v-if="isScanning" type="secondary">{{ scanStatus }}</a-text>
            </a-space>
          </a-col>
        </a-row>
        
        <a-table 
          :columns="scanColumns" 
          :data="scanResults" 
          :loading="isScanning" 
          :row-selection="{ type: 'checkbox', showCheckedAll: true, onlyCurrent: false }"
          v-model:selected-keys="selectedScanDevices"
          row-key="scan_row_key"
          size="small"
          :bordered="{ cell: true }"
          :pagination="false"
        >
          <template #status="{ record }">
            <a-tag v-if="record.diff_status === 'new'" color="green">New</a-tag>
            <a-tag v-else-if="record.diff_status === 'existing'" color="orange">Existing</a-tag>
            <a-tag v-else-if="record.diff_status === 'removed'" color="red">Removed</a-tag>
            <a-tag v-else-if="record.status === 'online'" color="green">Online</a-tag>
            <a-tag v-else color="gray">{{ record.status || '-' }}</a-tag>
          </template>
          <template #empty>
            <div v-if="isScanning" class="text-center py-8">
              <a-spin size="large" />
              <div class="mt-4 text-gray">{{ scanStatus }}</div>
              <div class="mt-2 text-gray text-sm">预计需要10秒左右</div>
            </div>
            <a-empty v-else description="暂无扫描结果" />
          </template>
        </a-table>
      </a-space>
    </a-modal>
  </div>
</template>

<script setup>
import { ref, reactive, onMounted, onUnmounted, computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { Message } from '@arco-design/web-vue'
import {
  IconArrowLeft, IconPlus, IconDelete, IconScan, IconList, IconThunderbolt,
  IconSettings, IconHistory, IconSearch, IconEye, IconLink,
  IconClockCircle
} from '@arco-design/web-vue/es/icon'
import request from '@/utils/request'
import { devicePointsRoutePath, channelDeviceApiPath } from '@/utils/deviceRoute'
import { generateShortId } from '@/utils/shortId'
import { formatProtocolTag } from '@/utils/protocolLabel'
import { postScanAndWait } from '@/utils/asyncJob'
import {
  formatListDetailMetaText,
  getListDetailMetaDotClass,
} from '@/utils/channelMetrics'
import { base64ToUint8Array, uint8ArrayToHex, detectFileType, downloadBytes } from '@/utils/decode'
import HistoryModal from '@/components/HistoryModal.vue'

const route = useRoute()
const router = useRouter()
const devices = ref([])
const channelInfo = ref(null)
const channelMetrics = ref(null)
const loading = ref(false)
const channelId = route.params.channelId
const channelProtocol = computed(() => channelInfo.value?.protocol || '')
const listDetailMetaText = computed(() => formatListDetailMetaText({
  channelId,
  deviceCount: devices.value.length,
  metrics: channelMetrics.value,
}))
const listDetailMetaDotClass = computed(() => getListDetailMetaDotClass(channelMetrics.value))
const channelOpcUaEndpoint = computed(() => {
  const cfg = channelInfo.value?.config || {}
  return cfg.url || cfg.endpoint || ''
})

const inheritOpcUaConfigFromChannel = (baseConfig = {}) => {
  const cfg = channelInfo.value?.config || {}
  const merged = { ...baseConfig }
  const channelEndpoint = cfg.url || cfg.endpoint || ''
  if (!merged.endpoint && channelEndpoint) {
    merged.endpoint = channelEndpoint
  }
  const inheritKeys = [
    'security_policy', 'security_mode', 'auth_method',
    'username', 'password', 'certificate_file', 'private_key_file'
  ]
  for (const key of inheritKeys) {
    if (merged[key] == null || merged[key] === '') {
      if (cfg[key] != null && cfg[key] !== '') {
        merged[key] = cfg[key]
      }
    }
  }
  return merged
}

const selected = ref([])
const selectAll = ref(false)
const dialog = ref(false)
const deleteDialog = ref(false)
const saving = ref(false)
const deleting = ref(false)
const isEdit = ref(false)
const itemToDelete = ref(null)

const ruleUsageDialog = reactive({
  show: false,
  deviceName: '',
  rules: []
})
const allRules = ref([])

const fetchRules = async () => {
  try {
    const data = await request.get('/api/edge/rules')
    allRules.value = data
  } catch (e) {
    console.error('Failed to fetch rules', e)
  }
}

const showRuleUsage = (device) => {
  ruleUsageDialog.deviceName = device.name
  ruleUsageDialog.rules = allRules.value.filter(rule => {
    if (rule.source && rule.source.device_id === device.id) return true
    if (rule.sources && rule.sources.some(s => s.device_id === device.id)) return true
    
    if (rule.actions) {
      return rule.actions.some(a => {
        if (a.config && a.config.device_id === device.id) return true
        if (a.config && a.config.targets && a.config.targets.some(t => t.device_id === device.id)) return true
        return false
      })
    }
    return false
  })
  ruleUsageDialog.show = true
}

const goToRule = (ruleId) => {
  router.push({ path: '/edge-compute', query: { rule: ruleId } })
}

const getDeviceStateColor = (state) => {
  switch (state) {
    case 0: return 'green'
    case 1: return 'orange'
    case 2: return 'red'
    case 3: return 'gray'
    default: return 'gray'
  }
}

const getDeviceStateText = (state) => {
  switch (state) {
    case 0: return '在线'
    case 1: return '不稳定'
    case 2: return '离线'
    case 3: return '隔离'
    default: return '未知'
  }
}

const formatFriendlyTime = (ts) => {
  if (!ts && ts !== 0) return '-'
  const date = new Date(Number(ts) * 1000)
  if (Number.isNaN(date.getTime())) return '-'

  const diff = Date.now() - date.getTime()
  const seconds = Math.floor(diff / 1000)
  const minutes = Math.floor(seconds / 60)
  const hours = Math.floor(minutes / 60)
  const days = Math.floor(hours / 24)

  if (seconds < 30) return '刚刚'
  if (seconds < 60) return `${seconds}秒前`
  if (minutes < 60) return `${minutes}分钟前`
  if (hours < 24) return `${hours}小时前`
  if (days < 7) return `${days}天前`

  const fmt = (n) => n.toString().padStart(2, '0')
  return `${date.getFullYear()}-${fmt(date.getMonth() + 1)}-${fmt(date.getDate())} ${fmt(date.getHours())}:${fmt(date.getMinutes())}:${fmt(date.getSeconds())}`
}

const getConfigValue = (device, field) => {
  if (!device || !device.config) return '-'
  let config = device.config
  if (typeof config === 'string') {
    try {
      config = JSON.parse(config)
    } catch (e) {
      return '-'
    }
  }
  if (field === 'instance_id') {
    return config.bacnet_device_id || '-'
  }
  if (field === 'ip') {
    return config.ip || '-'
  }
  if (field === 'vendor_model') {
    const vendor = config.vendor_name || '-'
    const model = config.model_name || '-'
    return `${vendor} / ${model}`
  }
  return config[field] || '-'
}

const defaultForm = {
  id: '',
  name: '',
  interval: '10s',
  enable: true,
  stationName: '',
  stationCode: '',
  roomName: '',
  roomCode: '',
  configStr: '{}',
  dlt645Address: '',
  dlt645AutoPointsEnabled: true,
  modbusSlaveId: 1,
  startAddressMode: 0,
  bacnet_device_id: '',
  ip: '',
  port: 47808,
  knxIndividualAddress: '',
  pnioDeviceName: '',
  pnioIp: '',
  pnioPort: 34964,
  pnioSlot: 0,
  pnioSubslot: 1,
  pnioApi: '',
  pnioIdent: '',
  pnioSubIdent: '',
  pnioProperties: '',
  pnioInputLength: 0,
  pnioOutputLength: 0,
  ecatPosition: 1,
  ecatVendorId: '',
  ecatProductCode: '',
  ecatTxPdoSize: 16,
  ecatRxPdoSize: 8,
  ecatAlias: 0,
  ecatRunMode: 'pdo',
  config: {},
  storageEnable: false,
  storageStrategy: 'interval',
  storageInterval: 1,
  storageMaxRecords: 1000,
  autoPointsEnabled: false,
  autoPointsStart: 0,
  autoPointsEnd: 199,
  autoPointsDatatype: 'int16',
  autoPointsReadWrite: 'R',
  autoPointsRegisterType: 'holding',
  regeneratePoints: false
}
const form = ref({ ...defaultForm })

const batchModbusDialog = ref(false)
const batchModbusLoading = ref(false)
const batchModbusRegisterTypes = [
  { label: '保持寄存器 (0x03)', value: 'holding' },
  { label: '输入寄存器 (0x04)', value: 'input' },
  { label: '输出线圈 (0x01)', value: 'coil' },
  { label: '离散输入 (0x02)', value: 'discrete' },
]

const batchModbusFunctionCodeMap = {
  coil: 1,
  discrete: 2,
  input: 4,
  holding: 3,
}

const batchModbusForm = ref({
  slaveStart: 1,
  slaveEnd: 7,
  regStart: 0,
  regEnd: 199,
  interval: '1s',
  datatype: 'int16',
  readwrite: 'R',
  register_type: 'holding',
  function_code: 3
})

const autoPointsCountLabel = computed(() => {
  const start = Number(form.value.autoPointsStart) || 0
  const end = Number(form.value.autoPointsEnd) || 0
  if (end < start) return '0'
  return String(end - start + 1)
})

const batchModbusPreview = computed(() => {
  const s1 = Number(batchModbusForm.value.slaveStart) || 0
  const s2 = Number(batchModbusForm.value.slaveEnd) || 0
  const r1 = Number(batchModbusForm.value.regStart) || 0
  const r2 = Number(batchModbusForm.value.regEnd) || 0
  const slaves = Math.max(0, Math.max(s1, s2) - Math.min(s1, s2) + 1)
  const points = Math.max(0, Math.max(r1, r2) - Math.min(r1, r2) + 1)
  return { slaves, points, total: slaves * points }
})

const openBatchModbusDialog = () => {
  batchModbusDialog.value = true
}

const onBatchRegisterTypeChange = (type) => {
  batchModbusForm.value.function_code = batchModbusFunctionCodeMap[type] || 3
}

const submitBatchModbus = async () => {
  batchModbusLoading.value = true
  try {
    const f = batchModbusForm.value
    const payload = {
      slave_start: f.slaveStart,
      slave_end: f.slaveEnd,
      reg_start: f.regStart,
      reg_end: f.regEnd,
      interval: f.interval,
      datatype: f.datatype,
      readwrite: f.readwrite,
      register_type: f.register_type,
      function_code: f.function_code
    }
    const res = await request.post(`/api/channels/${channelId}/devices/batch-modbus`, payload)
    const skipped = res.skipped?.length || 0
    let msg = `成功创建 ${res.created || 0} 台从站设备`
    if (skipped > 0) {
      msg += `，跳过已存在 ${skipped} 台`
    }
    if (res.warnings?.length) {
      Message.warning(msg + '（部分注册警告，见控制台）')
      console.warn('batch-modbus warnings', res.warnings)
    } else {
      Message.success(msg)
    }
    batchModbusDialog.value = false
    fetchDevices()
  } catch (e) {
    Message.error('批量创建失败: ' + (e.message || e))
    throw e
  } finally {
    batchModbusLoading.value = false
  }
}

const parseAutoPointsRange = (config) => {
  const rng = config?.auto_points_range
  if (!rng || typeof rng !== 'string') return null
  const parts = rng.split('-')
  if (parts.length !== 2) return null
  const start = parseInt(parts[0], 10)
  const end = parseInt(parts[1], 10)
  if (Number.isNaN(start) || Number.isNaN(end)) return null
  return { start, end }
}

const fetchDevices = async () => {
  loading.value = true
  try {
    const [chanData, devData, metricsData] = await Promise.all([
      request.get(`/api/channels/${channelId}`),
      request.get(`/api/channels/${channelId}/devices`, { params: { include_points: 'false' } }),
      request.get(`/api/channels/${channelId}/metrics`).catch(() => null),
    ])
    channelInfo.value = chanData
    devices.value = devData
    channelMetrics.value = metricsData

    selected.value = []
    selectAll.value = false
  } catch (e) {
    Message.error('获取设备失败: ' + e.message)
  } finally {
    loading.value = false
  }
}

const toggleSelectAll = (val) => {
  if (val) {
    selected.value = devices.value.map(d => d.id)
  } else {
    selected.value = []
  }
}

const openDialog = async (item = null) => {
  if (item) {
    let source = item
    if (!Array.isArray(item.points)) {
      try {
        source = await request.get(channelDeviceApiPath(channelId, item.id))
      } catch (e) {
        Message.error('获取设备详情失败: ' + e.message)
        return
      }
    }
    isEdit.value = true
    let config = source.config || {}
    if (typeof config === 'string') {
      try {
        config = JSON.parse(config)
      } catch (e) {
        config = {}
      }
    }
    const storage = item.storage || {}
    form.value = {
      ...source,
      config: config,
      configStr: JSON.stringify(config, null, 2),
      stationName: source.station_name || source.stationName || '',
      stationCode: source.station_code || source.stationCode || '',
      roomName: source.room_name || source.roomName || '',
      roomCode: source.room_code || source.roomCode || '',
      dlt645Address: config.station_address || config.address || '',
      dlt645AutoPointsEnabled: config.auto_points_enabled !== false,
      modbusSlaveId: config.slave_id || 1,
      startAddressMode: config.start_address || config.address_base || 0,
      bacnet_device_id: config.bacnet_device_id || '',
      ip: config.ip || '',
      port: config.port || 47808,
      knxIndividualAddress: config.knx_individual_addr || config.individual_address || '',
      pnioDeviceName: config.device_name || config.deviceName || '',
      pnioIp: config.ip || '',
      pnioPort: config.port || 34964,
      pnioSlot: config.slot ?? 0,
      pnioSubslot: config.subslot ?? config.sub_slot ?? 1,
      pnioApi: config.api || config.api_list || '',
      pnioIdent: config.ident || config.identifier || '',
      pnioSubIdent: config.sub_ident || config.subIdent || '',
      pnioProperties: config.properties || config.property || '',
      pnioInputLength: config.input_length || config.inputLength || 0,
      pnioOutputLength: config.output_length || config.outputLength || 0,
      storageEnable: storage.enable || false,
      storageStrategy: storage.strategy || 'interval',
      storageInterval: storage.interval || 1,
      storageMaxRecords: storage.max_records || 1000,
      autoPointsEnabled: !!config.auto_points_range,
      autoPointsStart: parseAutoPointsRange(config)?.start ?? 0,
      autoPointsEnd: parseAutoPointsRange(config)?.end ?? 199,
      autoPointsDatatype: config.auto_points_datatype || 'int16',
      autoPointsReadWrite: config.auto_points_readwrite || 'R',
      autoPointsRegisterType: config.auto_points_register_type || 'holding',
      regeneratePoints: false
    }
  } else {
    isEdit.value = false
    form.value = { ...defaultForm }
    if (channelProtocol.value && channelProtocol.value.includes('modbus')) {
      form.value.autoPointsEnabled = true
    }
    if (channelProtocol.value === 'dlt645') {
      form.value.dlt645AutoPointsEnabled = true
    }
    if (channelProtocol.value === 'opc-ua') {
      form.value.id = generateShortId()
      form.value.config = inheritOpcUaConfigFromChannel({
        security_policy: 'None',
        security_mode: 'None',
        auth_method: 'Anonymous'
      })
    }
  }
  dialog.value = true
}

const closeDialog = () => {
  dialog.value = false
  form.value = { ...defaultForm }
}

const saveDevice = async () => {
  let config = {}
  try {
    config = JSON.parse(form.value.configStr)
  } catch (e) {
    Message.error('配置参数必须是有效的JSON格式')
    return false
  }

  if (channelProtocol.value === 'dlt645') {
    config.station_address = form.value.dlt645Address
    config.address = form.value.dlt645Address
    config.auto_points_enabled = !!form.value.dlt645AutoPointsEnabled
  } else if (channelProtocol.value && channelProtocol.value.includes('modbus')) {
    config.slave_id = form.value.modbusSlaveId
    config.start_address = form.value.startAddressMode
    if (form.value.autoPointsEnabled) {
      const start = Number(form.value.autoPointsStart) || 0
      const end = Number(form.value.autoPointsEnd) || 0
      config.auto_points_range = `${Math.min(start, end)}-${Math.max(start, end)}`
      config.auto_points_datatype = form.value.autoPointsDatatype
      config.auto_points_readwrite = form.value.autoPointsReadWrite
      config.auto_points_register_type = form.value.autoPointsRegisterType
      config.auto_points_function_code = 3
    } else {
      delete config.auto_points_range
    }
  } else if (channelProtocol.value === 'bacnet-ip') {
    config.bacnet_device_id = parseInt(form.value.bacnet_device_id)
    if (form.value.ip) {
      config.ip = form.value.ip
    }
    if (form.value.port) {
      config.port = form.value.port
    }
  } else if (channelProtocol.value === 'opc-ua') {
    Object.assign(config, inheritOpcUaConfigFromChannel(form.value.config))
  } else if (channelProtocol.value === 'knxnet-ip') {
    const individualAddress = String(form.value.knxIndividualAddress || '').trim()
    if (individualAddress) {
      config.knx_individual_addr = individualAddress
    } else {
      delete config.knx_individual_addr
    }
  } else if (channelProtocol.value === 'profinet-io') {
    config.device_name = form.value.pnioDeviceName
    config.ip = form.value.pnioIp
    config.port = form.value.pnioPort || 34964
    config.slot = form.value.pnioSlot ?? 0
    config.subslot = form.value.pnioSubslot ?? 1
    if (form.value.pnioApi) config.api = form.value.pnioApi
    if (form.value.pnioIdent) config.ident = form.value.pnioIdent
    if (form.value.pnioSubIdent) config.sub_ident = form.value.pnioSubIdent
    if (form.value.pnioProperties) config.properties = form.value.pnioProperties
    if (form.value.pnioInputLength) config.input_length = form.value.pnioInputLength
    if (form.value.pnioOutputLength) config.output_length = form.value.pnioOutputLength
  } else if (channelProtocol.value === 'ethercat') {
    config.position = form.value.ecatPosition
    if (form.value.ecatVendorId) config.vendor_id = form.value.ecatVendorId
    if (form.value.ecatProductCode) config.product_code = form.value.ecatProductCode
    if (form.value.ecatTxPdoSize) config.tx_pdo_size = form.value.ecatTxPdoSize
    if (form.value.ecatRxPdoSize) config.rx_pdo_size = form.value.ecatRxPdoSize
    if (form.value.ecatAlias) config.alias = form.value.ecatAlias
    if (form.value.ecatRunMode) config.run_mode = form.value.ecatRunMode
  }

  const payload = {
    id: form.value.id,
    name: form.value.name,
    interval: form.value.interval,
    enable: form.value.enable,
    station_name: form.value.stationName || '',
    station_code: form.value.stationCode || '',
    room_name: form.value.roomName || '',
    room_code: form.value.roomCode || '',
    config: config,
    storage: {
      enable: form.value.storageEnable,
      strategy: form.value.storageStrategy,
      interval: form.value.storageInterval,
      max_records: form.value.storageMaxRecords
    },
  }
  
  // 编辑时后端 UpdateDevice 会自动保留原始点位，无需获取和发送
  // When editing, points are preserved by the backend UpdateDevice automatically.
  if (!isEdit.value) {
    payload.points = []
  }
  saving.value = true
  try {
    const url = isEdit.value
      ? channelDeviceApiPath(channelId, form.value.id)
      : `/api/channels/${channelId}/devices`
    const method = isEdit.value ? 'put' : 'post'
    
    await request({
      url: url,
      method: method,
      data: payload
    })

    if (isEdit.value && form.value.regeneratePoints && form.value.autoPointsEnabled) {
      const start = Number(form.value.autoPointsStart) || 0
      const end = Number(form.value.autoPointsEnd) || 0

      await request.post(channelDeviceApiPath(channelId, form.value.id, 'points', 'generate-registers'), {
        start: Math.min(start, end),
        end: Math.max(start, end),
        datatype: form.value.autoPointsDatatype,
        readwrite: form.value.autoPointsReadWrite,
        register_type: form.value.autoPointsRegisterType,
        function_code: 3,
        mode: 'merge'
      })
    }

    Message.success(isEdit.value ? '更新成功' : '创建成功')
    closeDialog()
    fetchDevices()
    return true
  } catch (e) {
    Message.error(e.message)
    return false
  } finally {
    saving.value = false
  }
}

const confirmDelete = (item) => {
  itemToDelete.value = item
  deleteDialog.value = true
}

const confirmBatchDelete = () => {
  itemToDelete.value = null
  deleteDialog.value = true
}

const executeDelete = async () => {
  if (deleting.value) return
  deleting.value = true
  try {
    if (itemToDelete.value) {
      await request.delete(channelDeviceApiPath(channelId, itemToDelete.value.id))
    } else {
      await request({
        url: `/api/channels/${channelId}/devices`,
        method: 'delete',
        data: selected.value
      })
    }
    
    Message.success('删除成功')
    deleteDialog.value = false
    fetchDevices()
  } catch (e) {
    Message.error(e.message)
  } finally {
    deleting.value = false
  }
}

const historyModalVisible = ref(false)
const historyDevice = ref(null)

const openHistoryDialog = (device) => {
  historyDevice.value = device
  historyModalVisible.value = true
}

const goToPoints = (device) => {
  router.push(devicePointsRoutePath(channelId, device.id))
}

const scanDialog = ref(false)
const isScanning = ref(false)
const scanResults = ref([])
const selectedScanDevices = ref([])
const selectAllScan = ref(false)
const isAddingDevices = ref(false)

const preconfiguredDevices = ref([])

const preconfigColumns = [
  { title: 'BACnet设备ID', slotName: 'bacnet_device_id', width: 140 },
  { title: 'IP地址', slotName: 'ip', width: 160 },
  { title: '端口', slotName: 'port', width: 100 },
  { title: '操作', slotName: 'action', width: 80 }
]

const scanStatus = ref('')
const scanTimeout = ref(null)

const scanRowKeyForItem = (item) => String(item?.endpoint || item?.bacnet_device_id || '')

const enrichScanResults = (items) => {
  // Build a set of existing device IDs for diff_status comparison
  const existingIds = new Set()
  ;(devices.value || []).forEach((d) => {
    if (d.config?.bacnet_device_id != null) {
      existingIds.add(Number(d.config.bacnet_device_id))
    }
  })

  return (items || []).map((item) => {
    const mappedDeviceId = item.bacnet_device_id ?? item.device_id ?? item.id
    return {
      ...item,
      // Map backend field names to frontend expectations
      bacnet_device_id: mappedDeviceId,
      vendor_name: item.vendor_name ?? (item.vendor_id != null ? `Vendor ${item.vendor_id}` : ''),
      model_name: item.model_name ?? item.object_name ?? '',
      diff_status: existingIds.has(Number(mappedDeviceId)) ? 'existing' : 'new',
      scan_row_key: String(item?.endpoint || mappedDeviceId || ''),
    }
  })
}

const normalizeScanResults = (res) => {
  let items = []
  if (Array.isArray(res)) items = res
  else if (res && Array.isArray(res.devices)) items = res.devices
  else if (res && Array.isArray(res.data)) items = res.data
  return enrichScanResults(items)
}

const addPreconfiguredDevice = () => {
  preconfiguredDevices.value.push({ bacnet_device_id: '', ip: '', port: 47808 })
}

const removePreconfiguredDevice = (index) => {
  preconfiguredDevices.value.splice(index, 1)
}

const getSelectedScanDevices = () => {
  const keySet = new Set((selectedScanDevices.value || []).map((k) => String(k)))
  return scanResults.value.filter((item) => keySet.has(item.scan_row_key))
}

const buildScannedDevicePayload = (scanItem) => {
  const instanceId = scanItem.bacnet_device_id
  let id = ''
  let name = ''
  let config = {}

  if (channelProtocol.value === 'bacnet-ip') {
    id = instanceId != null ? `bacnet-${instanceId}` : ''
    name = scanItem.object_name || scanItem.model_name || (instanceId != null ? String(instanceId) : '')
    config = {
      bacnet_device_id: instanceId,
      ip: scanItem.ip,
      port: scanItem.port,
      vendor_name: scanItem.vendor_name,
      model_name: scanItem.model_name,
    }
  } else if (channelProtocol.value === 'opc-ua') {
    const ep = scanItem.endpoint || channelOpcUaEndpoint.value || ''
    id = generateShortId()
    name = scanItem.name || scanItem.model_name || 'OPC UA Server'
    config = inheritOpcUaConfigFromChannel({
      endpoint: ep,
      name: scanItem.name,
      vendor_name: scanItem.vendor_name,
      model_name: scanItem.model_name,
      version: scanItem.version,
    })
  } else {
    id = instanceId != null ? String(instanceId) : (scanItem.id || '')
    name = scanItem.name || scanItem.object_name || id
  }

  return {
    id: String(id),
    name: String(name),
    interval: '10s',
    enable: true,
    config,
    points: [],
  }
}

const openScanDialog = () => {
  scanDialog.value = true
  scanResults.value = []
  selectedScanDevices.value = []
  selectAllScan.value = false
}

const scanDevices = async () => {
  isScanning.value = true
  scanResults.value = []
  selectedScanDevices.value = []
  scanStatus.value = '正在准备扫描...'
  
  let stopMessage = null
  stopMessage = Message.loading({
    content: '开始扫描设备，广播 WhoIs 发现所有 BACnet 设备...',
    duration: 0
  })
  
  const startTime = Date.now()
  
  if (scanTimeout.value) {
    clearInterval(scanTimeout.value)
  }
  
  scanTimeout.value = setInterval(() => {
    const elapsed = Math.round((Date.now() - startTime) / 1000)
    if (elapsed < 3) {
      scanStatus.value = '正在初始化扫描...'
    } else if (elapsed < 6) {
      scanStatus.value = '正在广播 WhoIs...'
    } else if (elapsed < 9) {
      scanStatus.value = '正在收集 IAm 响应...'
    } else {
      scanStatus.value = '正在整理扫描结果...'
    }
  }, 1000)
  
  try {
    console.log('开始扫描设备，channelId:', channelId)
    
    // Yabe 风格：后端绑定 0.0.0.0:47808 全网卡广播，无需前端传参
    const scanParams = {}
    if (channelProtocol.value === 'opc-ua') {
      const ep = channelOpcUaEndpoint.value
      if (ep) scanParams.endpoint = ep
    }
    
    const res = await postScanAndWait(request, `/api/channels/${channelId}/scan`, scanParams, {
      timeoutMs: 60000
    })
    
    console.log('扫描响应:', res)
    
    const scanTime = Math.round((Date.now() - startTime) / 1000)
    scanStatus.value = '扫描完成'
    
    scanResults.value = normalizeScanResults(res)
    selectedScanDevices.value = []
    if (stopMessage && typeof stopMessage === 'object' && typeof stopMessage.close === 'function') {
      stopMessage.close()
      stopMessage = null
    }
    Message.success({
      content: `扫描完成 (耗时 ${scanTime} 秒)，发现 ${scanResults.value.length} 个设备，查看结果`,
      duration: 3000
    })
  } catch (e) {
    console.error('扫描失败:', e)
    if (stopMessage && typeof stopMessage === 'object' && typeof stopMessage.close === 'function') {
      stopMessage.close()
      stopMessage = null
    }
    if (e.code === 'ECONNABORTED') {
      Message.error({
        content: '扫描超时，请检查网络连接或设备响应',
        duration: 3000
      })
    } else {
      Message.error({
        content: '扫描失败: ' + e.message,
        duration: 3000
      })
    }
  } finally {
    if (scanTimeout.value) {
      clearInterval(scanTimeout.value)
      scanTimeout.value = null
    }
    if (stopMessage && typeof stopMessage === 'object' && typeof stopMessage.close === 'function') {
      stopMessage.close()
      stopMessage = null
    }
    isScanning.value = false
    scanStatus.value = ''
  }
}

const addSelectedDevices = async () => {
  let devicesToAdd = getSelectedScanDevices().filter((item) => item.diff_status !== 'existing')
  if (devicesToAdd.length === 0) {
    const selected = getSelectedScanDevices()
    if (selected.length > 0) {
      Message.warning('所选设备已在通道中，无需重复添加')
    } else {
      Message.warning('请先选择要添加的设备')
    }
    return
  }

  isAddingDevices.value = true

  try {
    const payloads = devicesToAdd.map(buildScannedDevicePayload)
    for (const payload of payloads) {
      if (!payload.id || !payload.name) {
        console.error('Invalid scan payload', { devicesToAdd, payloads })
        throw new Error('扫描结果缺少设备标识，请重新扫描后重试')
      }
    }

    await request.post(`/api/channels/${channelId}/devices`, payloads)

    Message.success(`成功添加 ${payloads.length} 个设备`)
    scanDialog.value = false
    fetchDevices()
  } catch (e) {
    const msg = e?.response?.data?.error || e.message
    Message.error('添加设备失败: ' + msg)
  } finally {
    isAddingDevices.value = false
  }
}

const toggleDeviceStatus = async (record) => {
  record.statusLoading = true
  const previousValue = !record.enable
  try {
    // 只发送 enable 字段，避免将整个 record 对象（含 Vue 响应式代理）发送到后端
    // Only send the enable field to avoid sending the entire reactive proxy object.
    await request.put(channelDeviceApiPath(channelId, record.id), {
      enable: record.enable
    })
    Message.success('状态更新成功')
  } catch (e) {
    Message.error('状态更新失败: ' + e.message)
    record.enable = previousValue
  } finally {
    record.statusLoading = false
  }
}

const tableColumns = computed(() => {
  const columns = [
    { title: '设备名称 / 标识', slotName: 'name', width: 220 },
    { title: '位置信息', slotName: 'location', width: 200 },
    { title: '状态', slotName: 'enable', width: 88 },
    { title: '通信状态', slotName: 'state', width: 108 },
    { title: '采集间隔', slotName: 'interval', width: 108 },
  ]

  columns.push({ title: '操作', slotName: 'actions', width: 240, fixed: 'right' })
  
  return columns
})

const rowSelection = reactive({
  type: 'checkbox',
  showCheckedAll: true,
  onlyCurrent: false,
})

const scanColumns = computed(() => {
  if (channelProtocol.value === 'opc-ua') {
    return [
      { title: 'Endpoint', dataIndex: 'endpoint', width: 360 },
      { title: '名称', dataIndex: 'name', width: 200 },
      { title: '厂商', dataIndex: 'vendor_name', width: 200 },
      { title: '型号', dataIndex: 'model_name', width: 150 },
      { title: '版本', dataIndex: 'version', width: 100 },
      { title: '状态', slotName: 'status', width: 100 },
    ]
  } else {
    return [
      { title: 'BACnet设备ID', dataIndex: 'bacnet_device_id', width: 150 },
      { title: 'IP 地址', dataIndex: 'ip', width: 150 },
      { title: '端口', dataIndex: 'port', width: 100 },
      { title: '厂商', dataIndex: 'vendor_name', width: 200 },
      { title: '型号', dataIndex: 'model_name', width: 150 },
      { title: '对象名称', dataIndex: 'object_name', width: 200 },
      { title: '状态', slotName: 'status', width: 100 },
    ]
  }
})

onMounted(() => {
  fetchDevices()
  fetchRules()
})

onUnmounted(() => {
  // 清理扫描状态定时器，避免组件销毁后定时器泄漏导致页面卡死
  // Clean up scan interval to prevent timer leaks after component unmount.
  if (scanTimeout.value) {
    clearInterval(scanTimeout.value)
    scanTimeout.value = null
  }
})
</script>

<style scoped>
/* v3.0 — styles in src/styles/ */

.preconfigured-devices-section {
  margin-top: 8px;
}
.preconfigured-devices-section .section-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 8px;
}
.preconfigured-devices-section .section-title {
  font-weight: 500;
  font-size: 14px;
  color: var(--color-text-1);
}
</style>

