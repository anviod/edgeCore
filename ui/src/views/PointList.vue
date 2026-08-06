<template>
  <div class="page-shell point-list-container list-detail-page">
    <div class="page-header point-header list-page-header">
      <div class="header-left">
        <a-button type="outline" size="small" class="list-back-btn" @click="goBack">
          <template #icon><IconArrowLeft /></template>
          返回设备
        </a-button>
        <div class="header-info header-info--inline">
          <span class="protocol-tag protocol-tag--accent">{{ formatProtocolTag(channelProtocol) }}</span>
          <h2 class="page-title title-text">点位列表</h2>
        </div>
      </div>

      <div class="header-right header-actions point-header-actions">
        <a-space size="small" wrap>
          <a-button v-if="channelProtocol && channelProtocol.includes('modbus')" type="outline" status="warning" size="small" @click="openRegisterBlockDialog">
            <template #icon><IconPlus /></template>
            批量创建寄存器
          </a-button>
          <a-button type="outline" status="success" size="small" @click="openAddDialog">
            <template #icon><IconPlus /></template>
            新增点位
          </a-button>
          <a-button v-if="channelProtocol === 'bacnet-ip' || channelProtocol === 'opc-ua'" type="outline" size="small" @click="openDiscoverDialog">
            <template #icon><IconScan /></template>
            扫描点位
          </a-button>
          <a-button type="primary" size="small" @click="fetchPoints" :loading="loading">
            <template #icon><IconRefresh /></template>
            刷新
          </a-button>
          <a-button type="text" size="small" class="help-trigger-btn" @click="helpVisible = true">
            <template #icon><IconQuestionCircle /></template>
            配置指引
          </a-button>
        </a-space>
      </div>
    </div>

    <div class="point-list-toolbar">
      <div class="point-list-toolbar__filters">
        <a-input
          v-model="filters.search"
          placeholder="搜索点位 (ID/名称/地址)"
          size="small"
          allow-clear
          class="point-list-toolbar__search"
        >
          <template #prefix><IconSearch /></template>
        </a-input>
        <a-select
          v-model="filters.quality"
          :options="[{ label: 'Good', value: 'Good' }, { label: 'Bad', value: 'Bad' }]"
          placeholder="质量过滤"
          mode="multiple"
          size="small"
          class="point-list-toolbar__select point-list-toolbar__select--sm"
        />
      </div>
      <div v-if="selection.selectedIds.length > 0" class="point-list-toolbar__batch">
        <span class="point-list-toolbar__selected-count font-mono">已选 {{ selection.selectedIds.length }} 项</span>
        <a-button type="text" size="small" @click="selection.selectedIds = []">
          清除选择
        </a-button>
        <a-button type="outline" status="success" size="small" @click="goCreateVirtualShadow">
          <template #icon><IconThunderbolt /></template>
          拼虚拟设备 ({{ selection.selectedIds.length }})
        </a-button>
        <a-button status="danger" type="outline" size="small" @click="confirmBatchDelete">
          <template #icon><IconDelete /></template>
          批量删除 ({{ selection.selectedIds.length }})
        </a-button>
      </div>
    </div>

    <div class="list-detail-body">
      <a-spin :loading="loading" class="list-detail-spin">
        <div class="table-container">
          <div class="table-toolbar table-toolbar--points">
            <div class="left-title">{{ isModbusChannel ? 'MODBUS SLAVE VIEW' : 'POINT LIST' }}</div>
          </div>
          <ModbusSlavePointView
            v-if="isModbusChannel"
            :all-points="points"
            :points="filteredPoints"
            :selected-ids="selection.selectedIds"
            :filter-key="modbusFilterKey"
            :load-error="loadError"
            @selection-change="onModbusSelectionChange"
            @clear-filters="clearGlobalFilters"
            @retry="fetchPoints({ force: true })"
            @write="openWriteDialog"
            @edit="openEditDialog"
            @delete="confirmDelete"
            @debug="openDebug"
            @show-value="showFullValue"
          />
          <a-table
            v-else
            :columns="tableColumns"
            :data="filteredPoints"
            :row-selection="rowSelection"
            v-model:selected-keys="selection.selectedIds"
            row-key="id"
            class="industrial-table-fluid"
            :bordered="{ wrapper: true, cell: true }"
          >
            <template #value="{ record }">
              <a-tooltip :content="`${formatValue(record.value)} ${record.unit || ''} - ${getRegisterHint(record)}`">
                <div @click="showFullValue(record)" class="value-cell cursor-pointer block truncate">
                  <span class="value-text font-mono">{{ formatValue(record.value) }}</span>
                  <span v-if="record.unit" class="value-unit">{{ record.unit }}</span>
                </div>
              </a-tooltip>
            </template>

            <template #quality="{ record }">
              <span class="table-cell-semantic">
                <div class="status-display flex items-center">
                  <IconCheckCircle v-if="isQualityGood(record.quality)" class="mr-1 text-emerald-500" />
                  <IconCloseCircle v-else class="mr-1 text-red-500" />
                  <span class="font-mono text-xs">{{ record.quality }}</span>
                </div>
              </span>
            </template>

            <template #readwrite="{ record }">
              <div class="status-display flex items-center">
                <IconEdit v-if="record.readwrite === 'RW' || record.readwrite === 'W'" class="mr-1 text-blue-500" />
                <IconCheckCircle v-else class="mr-1 text-emerald-500" />
                <span class="font-mono text-xs">{{ record.readwrite }}</span>
              </div>
            </template>

            <template #timestamp="{ record }">
              <a-tooltip v-if="record?.updated_at" :content="`更新 ${formatDate(record.updated_at)}`">
                <div class="font-mono text-xs text-slate-500 cursor-default">
                  {{ record && (record.collected_at || record.timestamp) ? formatDate(record.collected_at || record.timestamp) : 'N/A' }}
                </div>
              </a-tooltip>
              <div v-else class="font-mono text-xs text-slate-500">
                {{ record && (record.collected_at || record.timestamp) ? formatDate(record.collected_at || record.timestamp) : 'N/A' }}
              </div>
            </template>

            <template #actions="{ record }">
              <div class="actions-container flex gap-1">
                <a-button
                  v-if="record.readwrite === 'RW' || record.readwrite === 'W'"
                  type="text"
                  size="mini"
                  class="hover:bg-slate-100"
                  @click="openWriteDialog(record)"
                >
                  写入
                </a-button>
                            
                <a-button
                  type="text"
                  size="mini"
                  class="hover:bg-slate-100"
                  @click="openEditDialog(record)"
                >
                  编辑
                </a-button>
                            
                <a-button
                  type="text"
                  size="mini"
                  status="danger"
                  class="hover:bg-red-50"
                  @click="confirmDelete(record)"
                >
                  删除
                </a-button>
                            
                <a-button
                  type="text"
                  size="mini"
                  status="info"
                  class="hover:bg-blue-50"
                  @click="openDebug(record)"
                >
                  调试
                </a-button>
              </div>
            </template>

            <template #empty>
              <div class="empty-state">
                <IconSearch size="48" class="text-slate-300" />
                <div class="empty-text font-mono">暂无匹配的点位数据</div>
                <div class="empty-actions">
                  <a-button v-if="points.length === 0" type="primary" size="small" @click="openCloneDialog">
                    <template #icon><IconCopy /></template>复制其它设备点位
                  </a-button>
                  <a-button v-else type="outline" size="small" @click="filters.search = ''; filters.quality = []">
                    清除过滤器
                  </a-button>
                </div>
              </div>
            </template>
          </a-table>
        </div>
      </a-spin>

      <!-- Connection Status Footer -->
      <div v-if="deviceInfo" class="terminal-info">
        <span class="terminal-dot"></span>
        <span class="monospace-text">
          连接状态:{{ deviceInfo.state === 0 ? '已连接' : deviceInfo.state === 1 ? '不稳定' : '已断开' }} | 协议: {{ formatProtocolTag(channelProtocol) }} | 连续通信:{{ deviceInfo.runtime?.success_count || 0 }} 次 | 最近失败:{{ deviceInfo.runtime?.last_fail_time && new Date(deviceInfo.runtime.last_fail_time).getFullYear() > 1 ? formatDate(deviceInfo.runtime.last_fail_time) : '无' }}
        </span>
      </div>
    </div>

    <a-modal v-model:visible="cloneDialog.visible" title="克隆其它设备点位" width="100%" :ok-loading="cloneDialog.loading" @ok="executeClone" @cancel="cloneDialog.visible = false">
      <a-space direction="vertical" :size="16" fill>
        <a-row :gutter="16">
          <a-col :span="8">
            <a-select
              v-model="cloneDialog.selectedChannel"
              :options="cloneDialog.channels"
              placeholder="选择通道"
              :loading="cloneDialog.loading"
              @change="onCloneChannelChange"
            />
          </a-col>
          <a-col :span="8">
            <a-select
              v-model="cloneDialog.selectedDevice"
              :options="cloneDialog.devices"
              placeholder="选择设备"
              :loading="cloneDialog.loading"
              @change="onCloneDeviceChange"
            />
          </a-col>
          <a-col :span="8">
            <a-input
              v-model="cloneDialog.search"
              placeholder="按名称或地址过滤"
              allow-clear
            >
              <template #prefix><IconSearch /></template>
            </a-input>
          </a-col>
        </a-row>
        <a-row :gutter="16" v-if="cloneDialog.points && cloneDialog.points.length > 0">
          <a-col :span="24">
            <a-space>
              <a-checkbox
                v-model="cloneDialog.selectAll"
                @change="toggleCloneSelectAll"
              >
                全选
              </a-checkbox>
              <span class="text-slate-600">
                已选择 {{ cloneDialog.selected.length }} / {{ cloneDialog.points.length }}
              </span>
            </a-space>
          </a-col>
        </a-row>
        <a-table
          :columns="cloneTableColumns"
          :data="filteredClonePoints"
          :pagination="false"
          :scroll="{ y: 360 }"
        >
          <template #checkbox="{ record }">
            <a-checkbox
              v-model="cloneDialog.selected"
              :value="record"
            />
          </template>
        </a-table>
        <a-empty v-if="!cloneDialog.loading && cloneDialog.points.length === 0" description="请选择通道与设备以加载点位" />
      </a-space>
    </a-modal>

    <!-- Point Config Dialog (Add/Edit) -->
    <a-modal 
      v-model:visible="pointDialog.visible" 
      :width="760"
      :mask-closable="false"
      unmount-on-close
      modal-class="channel-config-modal"
      :title="pointDialog.isEdit ? '编辑点位' : '新增点位'"
      @cancel="pointDialog.visible = false"
    >
      <a-form :model="pointDialog.form" layout="vertical" class="channel-config-form point-config-form form-controls-md">
        <div class="batch-form-fields">
          <div class="batch-form-row">
            <div class="form-field">
              <div class="field-label">点位 ID</div>
              <a-input
                v-model="pointDialog.form.id"
                placeholder="唯一标识符"
                :disabled="pointDialog.isEdit"
              />
            </div>
            <div class="form-field">
              <div class="field-label">点位名称</div>
              <a-input
                v-model="pointDialog.form.name"
                placeholder="点位名称"
              />
            </div>
          </div>

          <!-- Protocol Specific Configuration -->
          <ModbusPointConfig
            v-if="channelProtocol.startsWith('modbus')"
            v-model:form="pointDialog.form"
            :device-info="deviceInfo"
          />
                    
          <BacnetPointConfig
            v-else-if="channelProtocol === 'bacnet-ip'"
            v-model:form="pointDialog.form"
            :device-info="deviceInfo"
          />
                    
          <OpcuaPointConfig
            v-else-if="channelProtocol === 'opc-ua'"
            v-model:form="pointDialog.form"
            :device-info="deviceInfo"
          />

          <template v-else-if="channelProtocol === 'knxnet-ip'">
            <div class="modal-section__title modal-section__title--sub">KNX 组地址</div>
            <div class="advanced-block point-advanced-block">
              <div class="batch-form-row">
                <div class="form-field">
                  <div class="field-label">主组</div>
                  <a-input-number
                    v-model="pointDialog.knx.mainGroup"
                    :min="0"
                    :max="31"
                    :precision="0"
                    placeholder="0-31"
                    @change="updateKNXAddress"
                  />
                </div>
                <div class="form-field">
                  <div class="field-label">中组</div>
                  <a-input-number
                    v-model="pointDialog.knx.middleGroup"
                    :min="0"
                    :max="7"
                    :precision="0"
                    placeholder="0-7"
                    @change="updateKNXAddress"
                  />
                </div>
                <div class="form-field">
                  <div class="field-label">子组</div>
                  <a-input-number
                    v-model="pointDialog.knx.subGroup"
                    :min="0"
                    :max="255"
                    :precision="0"
                    placeholder="0-255"
                    @change="updateKNXAddress"
                  />
                </div>
              </div>
              <div class="batch-form-row">
                <div class="form-field">
                  <div class="field-label">个体地址（可选）</div>
                  <a-input
                    v-model="pointDialog.knx.individualAddress"
                    placeholder="例如 1.1.1"
                    @input="updateKNXAddress"
                  />
                </div>
                <div class="form-field">
                  <div class="field-label">位宽（可选）</div>
                  <a-input-number
                    v-model="pointDialog.knx.bitWidth"
                    :min="0"
                    :max="7"
                    :precision="0"
                    placeholder="0-7，0 为省略"
                    allow-clear
                    @change="updateKNXAddress"
                  />
                </div>
              </div>
              <div class="form-field">
                <div class="field-label">生成地址预览</div>
                <a-input
                  v-model="pointDialog.form.address"
                  readonly
                  placeholder="main/middle/sub"
                />
              </div>
            </div>
          </template>
                    
          <!-- Other Protocols -->
          <template v-else>
            <div class="form-field">
              <div class="field-label">{{ getProtocolAddressLabel() }}</div>
              <a-input
                v-model="pointDialog.form.address"
                :placeholder="getProtocolAddressPlaceholder()"
              />
            </div>
          </template>

          <template v-if="channelProtocol.startsWith('modbus')">
            <div class="modal-section__title modal-section__title--sub">高级设置</div>
            <div class="advanced-block point-advanced-block">
              <div class="point-advanced-fields">
                <div class="form-field">
                  <div class="field-label">数据格式</div>
                  <a-select
                    v-model="formatPresetSelected"
                    :options="filteredFormatPresets"
                    clearable
                    @update:value="onSelectFormatPreset"
                  />
                </div>
                <div class="batch-form-row">
                  <div class="form-field">
                    <div class="field-label">解析类型</div>
                    <a-select
                      v-model="pointDialog.parseType"
                      :options="filteredParseTypes"
                    />
                  </div>
                  <div class="form-field">
                    <div class="field-label">数据类型 (存储)</div>
                    <a-select
                      v-model="pointDialog.form.datatype"
                      :options="datatypeOptions"
                    />
                  </div>
                </div>
                <div class="batch-form-row">
                  <div class="form-field">
                    <div class="field-label">读公式模板</div>
                    <a-select
                      v-model="pointDialog.form.read_formula_template"
                      :options="formulaTemplates"
                      clearable
                      @update:value="onSelectFormulaTemplate('read')"
                    />
                  </div>
                  <div class="form-field">
                    <div class="field-label">读公式 (使用变量 v)</div>
                    <a-input
                      v-model="pointDialog.form.read_formula"
                      @input="validateFormula('read')"
                    />
                    <div v-if="formulaErrors.read" class="field-error">{{ formulaErrors.read }}</div>
                  </div>
                </div>
                <div class="batch-form-row">
                  <div class="form-field">
                    <div class="field-label">读写权限</div>
                    <a-select
                      v-model="pointDialog.form.readwrite"
                      :options="['R', 'RW']"
                    />
                  </div>
                  <div class="form-field">
                    <div class="field-label">写公式模板</div>
                    <a-select
                      v-model="pointDialog.form.write_formula_template"
                      :options="formulaTemplates"
                      clearable
                      @update:value="onSelectFormulaTemplate('write')"
                    />
                  </div>
                </div>
                <div class="batch-form-row">
                  <div class="form-field">
                    <div class="field-label">写公式 (使用变量 v)</div>
                    <a-input
                      v-model="pointDialog.form.write_formula"
                      @input="validateFormula('write')"
                    />
                    <div v-if="formulaErrors.write" class="field-error">{{ formulaErrors.write }}</div>
                  </div>
                  <div class="form-field">
                    <div class="field-label">单位</div>
                    <a-input v-model="pointDialog.form.unit" />
                  </div>
                </div>
                <div class="batch-form-row">
                  <div class="form-field">
                    <div class="field-label">缩放比例</div>
                    <a-input
                      v-model.number="pointDialog.form.scale"
                      type="number"
                      step="0.01"
                      placeholder="1.0"
                    />
                  </div>
                  <div class="form-field">
                    <div class="field-label">偏移量</div>
                    <a-input
                      v-model.number="pointDialog.form.offset"
                      type="number"
                      step="0.01"
                      placeholder="0"
                    />
                  </div>
                </div>
                <div class="form-field">
                  <div class="field-label">默认值</div>
                  <a-input v-model="pointDialog.defaultValue" />
                </div>
              </div>
              <div class="point-config-actions">
                <a-button type="outline" @click="openQuickValidate">
                  <template #icon><IconThunderbolt /></template>
                  快速验证
                </a-button>
                <a-button type="outline" @click="openTemplateDialog">
                  <template #icon><IconFile /></template>
                  协议模板
                </a-button>
              </div>
            </div>
          </template>
          <template v-else>
            <div class="modal-section__title modal-section__title--sub">数据解析</div>
            <div class="advanced-block point-advanced-block">
              <a-row :gutter="[24, 16]" class="field-grid">
                <a-col :span="24">
                  <a-form-item field="formatPreset" label="数据格式">
                    <a-select
                      v-model="formatPresetSelected"
                      :options="filteredFormatPresets"
                      clearable
                      @update:value="onSelectFormatPreset"
                    ></a-select>
                  </a-form-item>
                </a-col>
                <a-col :span="8">
                  <a-form-item field="byteLength" label="字节数">
                    <a-select
                      v-model="pointDialog.byteLength"
                      :options="[1, 2, 4, 8]"
                    ></a-select>
                  </a-form-item>
                </a-col>
                <a-col :span="8">
                  <a-form-item field="wordOrderOption" label="WordOrder(字序)">
                    <a-select
                      v-model="pointDialog.wordOrderOption"
                      :options="wordOrderOptionsForBytes"
                      :disabled="pointDialog.byteLength === 1"
                    ></a-select>
                  </a-form-item>
                </a-col>
                <a-col :span="8">
                  <a-form-item field="parseType" label="解析类型">
                    <a-select
                      v-model="pointDialog.parseType"
                      :options="parseTypesForBytes"
                    ></a-select>
                  </a-form-item>
                </a-col>
                <a-col :span="12">
                  <a-form-item field="datatype" label="数据类型(存储)">
                    <a-select
                      v-model="pointDialog.form.datatype"
                      :options="datatypeOptions"
                    ></a-select>
                  </a-form-item>
                </a-col>
                <a-col :span="12">
                  <a-form-item field="read_formula_template" label="读公式模板">
                    <a-select
                      v-model="pointDialog.form.read_formula_template"
                      :options="formulaTemplates"
                      clearable
                      @update:value="onSelectFormulaTemplate('read')"
                    ></a-select>
                  </a-form-item>
                </a-col>
                <a-col :span="12">
                  <a-form-item field="read_formula" label="读公式 (使用变量 v)" :error="formulaErrors.read">
                    <a-input
                      v-model="pointDialog.form.read_formula"
                      @input="validateFormula('read')"
                    ></a-input>
                  </a-form-item>
                </a-col>
                <a-col :span="12">
                  <a-form-item field="readwrite" label="读写权限">
                    <a-select
                      v-model="pointDialog.form.readwrite"
                      :options="['R', 'RW']"
                    ></a-select>
                  </a-form-item>
                </a-col>
                <a-col :span="12">
                  <a-form-item field="write_formula_template" label="写公式模板">
                    <a-select
                      v-model="pointDialog.form.write_formula_template"
                      :options="formulaTemplates"
                      clearable
                      @update:value="onSelectFormulaTemplate('write')"
                    ></a-select>
                  </a-form-item>
                </a-col>
                <a-col :span="12">
                  <a-form-item field="write_formula" label="写公式 (使用变量 v)" :error="formulaErrors.write">
                    <a-input
                      v-model="pointDialog.form.write_formula"
                      @input="validateFormula('write')"
                    ></a-input>
                  </a-form-item>
                </a-col>
                <a-col :span="12">
                  <a-form-item field="unit" label="单位">
                    <a-input
                      v-model="pointDialog.form.unit"
                    ></a-input>
                  </a-form-item>
                </a-col>
                <a-col :span="12">
                  <a-form-item field="scale" label="缩放比例">
                    <a-input-number
                      v-model="pointDialog.form.scale"
                      placeholder="缩放比例"
                      :step="0.01"
                      size="small"
                      :tooltip="{ title: '默认为 1.0', placement: 'top' }"
                    />
                  </a-form-item>
                </a-col>
                <a-col :span="12">
                  <a-form-item field="offset" label="偏移量">
                    <a-input-number
                      v-model="pointDialog.form.offset"
                      placeholder="偏移量"
                      :step="0.01"
                      size="small"
                      :tooltip="{ title: '默认为 0', placement: 'top' }"
                    />
                  </a-form-item>
                </a-col>
                <a-col :span="12">
                  <a-form-item field="defaultValue" label="默认值">
                    <a-input
                      v-model="pointDialog.defaultValue"
                      placeholder="默认值"
                      size="small"
                    />
                  </a-form-item>
                </a-col>
              </a-row>
              <div class="point-config-actions">
                <a-button type="outline" @click="openQuickValidate">
                  <template #icon><IconThunderbolt /></template>
                  快速验证
                </a-button>
                <a-button type="outline" @click="openTemplateDialog">
                  <template #icon><IconFile /></template>
                  协议模板
                </a-button>
              </div>
            </div>
          </template>
        </div>
      </a-form>

      <template #footer>
        <a-button @click="pointDialog.visible = false">取消</a-button>
        <a-button type="primary" :loading="pointDialog.loading" @click="submitPoint">
          保存配置
        </a-button>
      </template>
    </a-modal>

    <a-modal v-model="quickValidate.visible" max-width="640">
      <a-card>
        <a-card-title class="d-flex align-center">
          <span class="text-h6">快速验证当前解析配置</span>
          <div style="flex: 1;"></div>
          <a-tag
            v-if="quickValidate.status"
            :color="quickValidate.status === 'pass' ? 'success' : 'error'"
            size="small"
            class="mr-2"
          >
            {{ quickValidate.status === 'pass' ? '验证通过' : '未通过' }}
          </a-tag>
          <a-button type="text" size="small" @click="quickValidate.visible = false">
            <template #icon><IconClose /></template>
          </a-button>
        </a-card-title>
        <a-card-text>
          <a-row :gutter="16" class="dense">
            <a-col :span="24">
              <a-input
                v-model="quickValidate.rawHex"
                placeholder="原始十六进制报文 (例如: 01 0A FF 00)"
                size="small"
              />
            </a-col>
            <a-col :span="24">
              <a-input
                v-model="quickValidate.registerValues"
                placeholder="寄存器值列表(可选, 以空格或逗号分隔, 支持0x前缀) 例如: 0x1234 0x5678 或 4660 22136"
                size="small"
              />
            </a-col>
            <a-col :span="24" :sm="12">
              <a-input
                v-model="quickValidate.registerBaseAddress"
                placeholder="起始寄存器地址(仅用于标注) 例如: 40001"
                size="small"
              />
            </a-col>
            <a-col :span="24">
              <a-input
                v-model="quickValidate.expected"
                placeholder="期望工程值(可选) 例如: 230.1 或 LongABCD 11112222"
                size="small"
              />
            </a-col>
            <a-col :span="24">
              <div class="text-caption mb-1">解析结果预览</div>
              <div class="pa-3 rounded bg-grey-lighten-4 font-mono text-body-2">
                <span v-html="quickValidate.previewHtml"></span>
              </div>
            </a-col>
          </a-row>
        </a-card-text>
        <div class="pa-4 pt-0" style="display: flex; justify-content: flex-end; border-top: 1px solid #e8e8e8;">
          <div style="flex: 1;"></div>
          <a-button variant="text" @click="quickValidate.visible = false">关闭</a-button>
          <a-button color="primary" variant="elevated" @click="runQuickValidate">
            立即验证
          </a-button>
          <a-button
            color="secondary"
            variant="tonal"
            :disabled="quickValidate.status !== 'pass'"
            @click="saveCurrentAsTemplate"
          >
            保存为模板
          </a-button>
        </div>
      </a-card>
    </a-modal>

    <a-modal v-model="templateDialog.visible" max-width="900">
      <a-card>
        <a-card-title class="d-flex align-center">
          <span class="text-h6">协议模板示例</span>
          <div style="flex: 1;"></div>
          <a-input
            v-model="templateDialog.search"
            prepend-inner-icon="mdi-magnify"
            label="搜索模板(名称/描述)"
            variant="outlined"
            density="compact"
            hide-details
            style="max-width: 260px"
          ></a-input>
          <a-button type="text" size="small" @click="templateDialog.visible = false">
            <template #icon><IconClose /></template>
          </a-button>
        </a-card-title>
        <a-card-text>
          <a-row :gutter="16" class="dense">
            <a-col
              v-for="tpl in filteredPointTemplates"
              :key="tpl.id"
              cols="12"
              md="6"
            >
              <a-card variant="outlined" class="pa-3">
                <div class="d-flex align-center mb-1">
                  <span class="font-weight-medium">{{ tpl.name }}</span>
                  <div style="flex: 1;"></div>
                  <a-tag size="x-small" color="primary" class="mr-1">
                    {{ tpl.protocol }}
                  </a-tag>
                </div>
                <div class="text-caption text-grey-darken-1 mb-2">
                  {{ tpl.description }}
                </div>
                <div class="text-caption mb-1">
                  类型: {{ tpl.parseType }} / {{ tpl.datatype }},
                  字节数: {{ tpl.byteLength || 'N/A' }},
                  字序: {{ tpl.wordOrder || 'N/A' }},
                  单位: {{ tpl.unit || '-' }},
                  默认值: {{ tpl.defaultValue === '' ? '-' : tpl.defaultValue }},
                  权限: {{ tpl.readwrite }}
                </div>
                <div class="mt-2 d-flex">
                  <a-button
                    color="primary"
                    size="small"
                    variant="elevated"
                    prepend-icon="mdi-clipboard-arrow-right"
                    @click="applyTemplate(tpl)"
                  >
                    套用模板
                  </a-button>
                  <a-button
                    class="ml-2"
                    color="secondary"
                    size="small"
                    variant="text"
                    prepend-icon="mdi-content-copy"
                    @click="copyTemplate(tpl)"
                  >
                    复制配置
                  </a-button>
                </div>
              </a-card>
            </a-col>
          </a-row>
        </a-card-text>
        <div class="pa-4 pt-0" style="display: flex; justify-content: flex-end; border-top: 1px solid #e8e8e8;">
          <div style="flex: 1;"></div>
          <a-button type="primary" size="small" @click="templateDialog.visible = false">关闭</a-button>
        </div>
      </a-card>
    </a-modal>

    <a-modal v-model="helpDialog.visible" max-width="900">
      <a-card>
        <a-card-title class="d-flex align-center">
          <span class="text-h6">点位解码与公式使用帮助</span>
          <div style="flex: 1;"></div>
          <a-button type="text" size="small" @click="helpDialog.visible = false">
            <template #icon><IconClose /></template>
          </a-button>
        </a-card-title>
        <a-card-text class="pt-2">
          <a-input
            v-model="helpDialog.search"
            prepend-inner-icon="mdi-magnify"
            label="搜索关键字 (协议 / 函数 / 示例)"
            variant="outlined"
            density="compact"
            class="mb-4"
            clearable
          ></a-input>
          <a-collapse multiple>
            <a-collapse-item
              v-for="section in filteredHelpSections"
              :key="section.id"
              :title="section.title"
            >
              <div>
                <div v-for="item in section.items" :key="item.title" class="mb-4">
                  <div class="d-flex align-center mb-1">
                    <span class="font-weight-medium">{{ item.title }}</span>
                    <div style="flex: 1;"></div>
                    <a-button
                      v-if="item.snippet"
                      type="text"
                      size="small"
                      @click="copySnippet(item.snippet)"
                    >
                      复制示例
                    </a-button>
                  </div>
                  <div class="text-body-2 mb-1">{{ item.desc }}</div>
                  <div v-if="item.snippet" class="pa-2 rounded bg-grey-lighten-4 font-mono text-body-2">
                    {{ item.snippet }}
                  </div>
                </div>
              </div>
            </a-collapse-item>
          </a-collapse>
        </a-card-text>
        <div class="pa-4" style="display: flex; justify-content: flex-end; border-top: 1px solid #e8e8e8;">
          <div style="flex: 1;"></div>
          <a-button type="primary" size="small" @click="helpDialog.visible = false">关闭</a-button>
        </div>
      </a-card>
    </a-modal>

    <!-- BACnet Scanner Dialog -->
    <BACnetScanner
      v-if="channelProtocol === 'bacnet-ip'"
      :visible="scanDialogVisible"
      :channel-id="channelId"
      :device-id="deviceId"
      :channel-protocol="channelProtocol"
      :existing-addresses="existingAddresses"
      :device-info="deviceInfo"
      @close="scanDialogVisible = false"
      @refresh-points="handleRefreshPoints"
    />


    <a-modal
      v-model:visible="registerBlockDialog.visible"
      title="批量创建寄存器区块"
      :width="760"
      modal-class="channel-config-modal channel-config-modal--batch"
      :ok-loading="registerBlockDialog.loading"
      ok-text="生成点位"
      @ok="submitRegisterBlock"
      @cancel="registerBlockDialog.visible = false"
    >
      <p class="modal-intro modal-intro--compact">
        按地址区间批量生成 Modbus 点位。合并模式下保留同 ID 现有点位配置。
      </p>
      <a-form layout="vertical" class="channel-config-form batch-modbus-form form-controls-md">
        <div class="batch-form-fields">
          <div class="form-field">
            <div class="field-label">地址区间</div>
            <div class="batch-range-control">
              <a-input-number
                v-model="registerBlockDialog.start"
                :min="0"
                :max="65535"
                placeholder="0"
              />
              <span class="batch-range-sep">至</span>
              <a-input-number
                v-model="registerBlockDialog.end"
                :min="0"
                :max="65535"
                placeholder="199"
              />
            </div>
          </div>

          <div class="batch-form-row">
            <div class="form-field">
              <div class="field-label">寄存器类型</div>
              <a-select
                v-model="registerBlockDialog.register_type"
                :options="registerBlockRegisterTypes"
                @change="onRegisterBlockTypeChange"
              />
            </div>
            <div class="form-field">
              <div class="field-label">数据类型</div>
              <a-select
                v-model="registerBlockDialog.datatype"
                :options="registerBlockDatatypeOptions"
                placeholder="选择数据类型"
              />
            </div>
          </div>

          <div class="form-field">
            <div class="field-label">读写</div>
            <a-select
              v-model="registerBlockDialog.readwrite"
              :options="registerBlockReadWriteOptions"
            />
          </div>

          <div class="form-field">
            <div class="field-label">生成模式</div>
            <a-radio-group v-model="registerBlockDialog.mode" direction="vertical" class="register-block-mode">
              <a-radio value="merge">合并（保留现有点）</a-radio>
              <a-radio value="replace">替换（仅保留新区间）</a-radio>
            </a-radio-group>
          </div>

          <div class="batch-preview-block">
            <div class="field-label batch-preview-block__label">创建预览</div>
            <div class="batch-preview-stats batch-preview-stats--single">
              <div class="batch-preview-stat batch-preview-stat--accent">
                <span class="batch-preview-stat__value">{{ registerBlockCount }}</span>
                <span class="batch-preview-stat__label">点位</span>
              </div>
            </div>
          </div>
        </div>
      </a-form>
    </a-modal>

    <!-- Delete Confirmation Dialog -->
    <a-modal v-model:visible="deleteDialog.visible" width="400px" @ok="executeDelete" @cancel="deleteDialog.visible = false">
      <template #title>
        <div class="flex items-center gap-2">
          <IconCloseCircle class="text-red-500" />
          <span>确认删除</span>
        </div>
      </template>
      <div class="text-center py-4">
        <template v-if="deleteDialog.isBatch">
          确定要批量删除选中的 <span class="text-red-500 font-bold">{{ deleteDialog.batchCount }}</span> 个点位吗？
        </template>
        <template v-else>
          确定要删除点位 <span class="text-red-500 font-bold">{{ deleteDialog.point?.name || deleteDialog.point?.id }}</span> 吗？
        </template>
        <div class="mt-2 text-gray-400 text-sm">此操作不可撤销。</div>
      </div>
      <template #footer>
        <div class="flex justify-end gap-2">
          <a-button @click="deleteDialog.visible = false">取消</a-button>
          <a-button type="primary" status="danger" @click="executeDelete" :loading="deleteDialog.loading">确认删除</a-button>
        </div>
      </template>
    </a-modal>

    <!-- Debug Dialog -->
    <a-modal
      v-model:visible="debugDialog.visible"
      title="点位调试"
      width="640px"
      @cancel="debugDialog.visible = false"
    >
      <div class="debug-dialog-meta">
        <span>{{ debugDialog.pointName }}</span>
      </div>
      <pre class="debug-dialog-content">{{ debugDialog.content }}</pre>
      <template #footer>
        <a-button type="primary" @click="debugDialog.visible = false">关闭</a-button>
      </template>
    </a-modal>

    <!-- Write Dialog -->
    <a-modal 
      v-model:visible="writeDialog.visible" 
      :width="480"
      :mask-closable="false"
      unmount-on-close
      modal-class="write-value-modal"
      title-align="start"
      @cancel="writeDialog.visible = false"
      @ok="submitWrite"
    >
      <template #title>
        <div style="display: flex; align-items: center; gap: 12px;">
          <span style="font-size: 16px; font-weight: 700; color: var(--text-primary);">写入数值</span>
          <span style="font-size: 11px; color: var(--text-secondary); font-family: monospace; text-transform: uppercase;">{{ getDataTypeLabel() }}</span>
        </div>
      </template>

      <div style="padding: 16px;">
        <!-- 设备信息 -->
        <div style="display: flex; gap: 16px; margin-bottom: 16px; padding: 10px; background: var(--edgeCore-surface-inset); border: 1px solid var(--edgeCore-border);">
          <div>
            <div style="font-size: 10px; color: var(--text-secondary); text-transform: uppercase;">设备</div>
            <div style="font-size: 13px; font-weight: 600; font-family: monospace;">{{ writeDialog.deviceID }}</div>
          </div>
          <div style="border-left: 1px solid var(--edgeCore-border);"></div>
          <div>
            <div style="font-size: 10px; color: var(--text-secondary); text-transform: uppercase;">点位</div>
            <div style="font-size: 13px; font-weight: 600; font-family: monospace;">{{ writeDialog.pointID }}</div>
          </div>
        </div>

        <!-- Boolean 类型 -->
        <template v-if="isBoolType(writeDialog.dataType)">
          <div style="margin-bottom: 8px; font-size: 12px; color: var(--text-secondary);">BOOL 布尔值</div>
          <a-radio-group v-model="writeDialog.valueBool" type="button">
            <a-radio :value="true"><strong>TRUE</strong> 真</a-radio>
            <a-radio :value="false"><strong>FALSE</strong> 假</a-radio>
          </a-radio-group>
        </template>

        <!-- String 类型 -->
        <template v-else-if="isStringType(writeDialog.dataType)">
          <div style="margin-bottom: 8px; font-size: 12px; color: var(--text-secondary);">STRING 字符串</div>
          <a-input
            v-model="writeDialog.valueStr"
            :placeholder="getStringInputPlaceholder()"
            size="large"
            autofocus
          />
        </template>

        <!-- ByteString (二进制) 类型 -->
        <template v-else-if="isBinaryType(writeDialog.dataType)">
          <div style="margin-bottom: 8px; font-size: 12px; color: var(--text-secondary);">BYTE STRING 二进制</div>
          <a-radio-group v-model="writeDialog.binaryEncoding" type="button" size="small">
            <a-radio value="hex">Hex</a-radio>
            <a-radio value="base64">Base64</a-radio>
          </a-radio-group>
          <a-input
            v-model="writeDialog.valueBinary"
            :placeholder="getBinaryWritePlaceholder()"
            size="large"
            style="margin-top: 8px; font-family: monospace;"
            autofocus
          />
          <div style="margin-top: 6px; font-size: 11px; color: var(--text-secondary);">示例: {{ getBinaryWriteHint() }}</div>
        </template>

        <!-- DateTime 类型 -->
        <template v-else-if="isDateTimeType(writeDialog.dataType)">
          <div style="margin-bottom: 8px; font-size: 12px; color: var(--text-secondary);">DATETIME 日期时间</div>
          <a-date-picker
            v-model="writeDialog.valueDateTime"
            show-time
            format="YYYY-MM-DD HH:mm:ss"
            size="large"
            style="width: 100%"
            autofocus
          />
        </template>

        <!-- Guid 类型 -->
        <template v-else-if="isGuidType(writeDialog.dataType)">
          <div style="margin-bottom: 8px; font-size: 12px; color: var(--text-secondary);">GUID 全局唯一标识符</div>
          <a-input
            v-model="writeDialog.valueStr"
            placeholder="12345678-1234-1234-1234-123456789abc"
            size="large"
            style="font-family: monospace;"
            autofocus
          />
        </template>

        <!-- StatusCode 类型 -->
        <template v-else-if="isStatusCodeType(writeDialog.dataType)">
          <div style="margin-bottom: 8px; font-size: 12px; color: var(--text-secondary);">STATUS CODE 状态码</div>
          <a-select
            v-model="writeDialog.valueStatusCode"
            :options="statusCodeOptions"
            placeholder="选择状态码"
            size="large"
            allow-search
            style="width: 100%"
          />
        </template>

        <!-- QualifiedName 类型 -->
        <template v-else-if="isQualifiedNameType(writeDialog.dataType)">
          <div style="margin-bottom: 8px; font-size: 12px; color: var(--text-secondary);">QUALIFIED NAME 限定名</div>
          <a-input
            v-model="writeDialog.valueStr"
            placeholder="命名空间:名称 (如 2:Temperature)"
            size="large"
            style="font-family: monospace;"
            autofocus
          />
        </template>

        <!-- LocalizedText 类型 -->
        <template v-else-if="isLocalizedTextType(writeDialog.dataType)">
          <div style="margin-bottom: 8px; font-size: 12px; color: var(--text-secondary);">LOCALIZED TEXT 本地化文本</div>
          <a-input
            v-model="writeDialog.valueStr"
            placeholder="请输入文本"
            size="large"
            autofocus
          />
        </template>

        <!-- NodeId 类型 -->
        <template v-else-if="isNodeIdType(writeDialog.dataType)">
          <div style="margin-bottom: 8px; font-size: 12px; color: var(--text-secondary);">NODE ID 节点标识符</div>
          <a-input
            v-model="writeDialog.valueStr"
            placeholder="ns=2;s=Node 或 i=2255"
            size="large"
            style="font-family: monospace;"
            autofocus
          />
        </template>

        <!-- 数值类型 (Int, Float, Double) -->
        <template v-else>
          <div style="margin-bottom: 8px; font-size: 12px; color: var(--text-secondary);">
            {{ getNumericTypeInfo() || '数值' }}
          </div>
          <a-input
            v-model="writeDialog.valueStr"
            placeholder="如 3.14 或 1e-5"
            size="large"
            autofocus
            @keydown.enter="submitWrite"
          />
          <div style="margin-top: 6px; font-size: 11px; color: var(--text-secondary);">支持整数、小数、科学计数法</div>
        </template>
      </div>

      <template #footer>
        <div style="display: flex; justify-content: flex-end; gap: 8px;">
          <a-button @click="writeDialog.visible = false">取消</a-button>
          <a-button 
            type="primary" 
            status="warning" 
            :loading="writeDialog.loading" 
            @click="submitWrite"
          >
            立即下发
          </a-button>
        </div>
      </template>
    </a-modal>

    <!-- Value Detail Dialog -->
    <a-modal v-model:visible="valueDialog.visible" title="完整数值" width="700px">
      <a-card>
        <div class="pt-4">
          <a-textarea
            placeholder="原始值"
            v-model="valueDialog.value"
            :read-only="true"
            :auto-size="{ minRows: 3, maxRows: 10 }"
            class="mb-4"
          />


          <div v-if="valueDialog.isBase64">
            <div class="text-subtitle-1 mb-2 font-weight-bold">Base64 解码</div>
            <a-radio-group v-model="valueDialog.decodeType" @change="tryDecode" class="mb-4">
              <a-radio value="text">Text (UTF-8)</a-radio>
              <a-radio value="hex">Hex</a-radio>
              <a-radio value="json">JSON</a-radio>
            </a-radio-group>
                        
            <a-textarea
              placeholder="解码结果"
              v-model="valueDialog.decodedValue"
              :read-only="true"
              :auto-size="{ minRows: 5, maxRows: 10 }"
              style="font-family: monospace;"
            />
          </div>
          <div v-else-if="numericFormats">
            <div class="text-subtitle-1 mb-3 font-weight-bold">数值格式转换</div>
            <a-row class="mb-3">
              <a-col :span="12" :sm="6">
                <div class="text-body-2 text-grey-darken-1 mb-1">字节数</div>
                <a-button-group>
                  <a-button :type="valueDialog.byteLength === 1 ? 'primary' : 'outline'" size="small" @click="valueDialog.byteLength = 1">1 字节</a-button>
                  <a-button :type="valueDialog.byteLength === 2 ? 'primary' : 'outline'" size="small" @click="valueDialog.byteLength = 2">2 字节</a-button>
                  <a-button :type="valueDialog.byteLength === 4 ? 'primary' : 'outline'" size="small" @click="valueDialog.byteLength = 4">4 字节</a-button>
                  <a-button :type="valueDialog.byteLength === 8 ? 'primary' : 'outline'" size="small" @click="valueDialog.byteLength = 8">8 字节</a-button>
                </a-button-group>
              </a-col>
              <a-col :span="12" :sm="6" v-if="valueWordOrderOptions.length">
                <div class="text-body-2 text-grey-darken-1 mb-1">字节 / 字序</div>
                <a-select
                  v-model="valueDialog.wordOrder"
                  :options="valueWordOrderOptions"
                  :field-names="{ label: 'label', value: 'value' }"
                  size="small"
                  style="width: 100%;"
                />
              </a-col>
            </a-row>
            <a-table :columns="valueFormatColumns" :data="valueFormatData" size="small" :bordered="true">
            </a-table>
          </div>
        </div>
        <div class="dialog-footer" style="display: flex; justify-content: flex-end; padding: 16px; border-top: 1px solid #e8e8e8;">
          <a-button type="primary" @click="valueDialog.visible = false">关闭</a-button>
        </div>
      </a-card>
    </a-modal>

    <!-- Help Drawer -->
    <HelpDrawer
      v-model:visible="helpVisible"
      :channel-protocol="channelProtocol"
    />

    <!-- OPC-UA Scanner -->
    <OpcuaScanner
      v-model:visible="opcuaScannerVisible"
      :channel-id="channelId"
      :device-id="deviceId"
      :device-config="deviceInfo?.config || {}"
      :channel-config="channelInfo?.config || {}"
      :existing-points="points"
      @error="(message) => showMessage(message, 'error')"
      @info="(message) => showMessage(message, 'info')"
      @points-added="handlePointsAdded"
    />
  </div>
</template>

<script setup>
import { ref, reactive, onMounted, onUnmounted, computed, watch, nextTick, h } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import { globalState, showMessage } from '../composables/useGlobalState'
import request from '@/utils/request'
import basePointTemplates from '@/utils/pointTemplates.json'
import { sanitizeHtml } from '@/utils/sanitizeHtml'
import {
  IconArrowLeft, IconSearch, IconPlus, IconDelete, IconRefresh,
  IconEdit, IconFile, IconBug, IconCheckCircle, IconClockCircle,
  IconCloseCircle, IconScan, IconCopy, IconClose, IconThunderbolt,
  IconTag, IconFolder, IconSend, IconQuestionCircle,
} from '@arco-design/web-vue/es/icon'
import { Message } from '@arco-design/web-vue'
import HelpDrawer from '../components/HelpDrawer.vue'
import OpcuaScanner from '../components/OpcuaScanner.vue'
import BACnetScanner from '../components/BACnetScanner.vue'
import ModbusSlavePointView from '../components/ModbusSlavePointView.vue'
import { enrichModbusPoint } from '@/utils/modbusRegisterGroups'
import { formatProtocolTag } from '@/utils/protocolLabel'
import ModbusPointConfig from '../components/point-config/ModbusPointConfig.vue'
import OpcuaPointConfig from '../components/point-config/OpcuaPointConfig.vue'
import BacnetPointConfig from '../components/point-config/BacnetPointConfig.vue'


import {
    baseWordOrderOptions,
    baseParseTypeOptions,
    getWordOrderOptionsForBytes,
    filterParseTypesByBytes,
    wordOrderToBackend,
    reorderBytes,
    parseByType,
    applyFormula,
    registersToBytes
} from '@/utils/pointDecodeHelper'
import {
    decodePathSegment,
    channelDeviceApiPath,
    devicePointsRoutePath,
    encodePathSegment,
} from '@/utils/deviceRoute'

const { locale } = useI18n()
const currentLang = computed(() => (locale.value || 'zh').toString())

const route = useRoute()
const router = useRouter()
const points = ref([])
const deviceInfo = ref(null)
const loading = ref(false)
const loadError = ref('')

const goBack = () => {
  console.log('goBack function called')
  // 使用router.push()返回上一页，更加可靠
  router.push(`/channels/${channelId.value}/devices`)
}

const goCreateVirtualShadow = () => {
  if (selection.selectedIds.length === 0) return
  const refs = selection.selectedIds
    .map(pid => `${channelId.value}.${deviceId.value}.${pid}`)
    .join(',')
  router.push({
    path: '/virtual-shadows',
    query: { new: '1', channel_id: channelId.value, refs }
  })
}

const channelId = computed(() => decodePathSegment(route.params.channelId))
const deviceId = computed(() => decodePathSegment(route.params.deviceId))

// Watch for route changes to refresh data
watch([channelId, deviceId], async () => {
    selection.selectedIds = []
    await fetchChannel()
    fetchPoints({ force: true })
    fetchMetrics()
})

// Point Filtering & Selection
const filters = reactive({
    search: '',
    quality: [],
})

const modbusFilterKey = computed(() => JSON.stringify([
    filters.search.trim().toLowerCase(),
    [...filters.quality].sort(),
]))

const clearGlobalFilters = () => {
    filters.search = ''
    filters.quality = []
}

const isModbusChannel = computed(() => {
    const proto = channelProtocol.value || ''
    return proto.startsWith('modbus') || proto.includes('modbus')
})

const onModbusSelectionChange = (ids) => {
    selection.selectedIds = [...new Set(ids)]
    selection.selectAll = selection.selectedIds.length > 0
        && selection.selectedIds.length === filteredPoints.value.length
}

const mapDevicePoint = (p, now = new Date()) => enrichModbusPoint({
    id: p.id,
    name: p.name,
    address: p.address,
    datatype: p.datatype || p.dataType,
    unit: p.unit || '',
    readwrite: p.readwrite || 'R',
    register_type: p.register_type,
    function_code: p.function_code,
    slave_id: p.slave_id,
    value: p.value ?? null,
    quality: p.quality || 'Bad',
    timestamp: p.timestamp || p.collected_at || now,
    collected_at: p.collected_at,
    updated_at: p.updated_at
})

const selection = reactive({
    selectedIds: [],
    selectAll: false
})

// Help Drawer State
const helpVisible = ref(false)

// OPC-UA Scanner State
const opcuaScannerVisible = ref(false)

const rowSelection = reactive({
    type: 'checkbox',
    showCheckedAll: true,
    onlyCurrent: false,
})

// Helper functions
const formatValue = (val) => {
    if (val === null || val === undefined) return 'N/A'
    if (typeof val === 'number') return val.toFixed(2)
    return val
}

const formatDate = (ts) => {
    if (!ts) return 'N/A'
    
    try {
        const date = new Date(ts)
        // 检查日期是否有效
        if (isNaN(date.getTime())) {
            return 'N/A'
        }
        
        const year = date.getFullYear()
        const month = String(date.getMonth() + 1).padStart(2, '0')
        const day = String(date.getDate()).padStart(2, '0')
        const hours = String(date.getHours()).padStart(2, '0')
        const minutes = String(date.getMinutes()).padStart(2, '0')
        const seconds = String(date.getSeconds()).padStart(2, '0')
        
        return `${year}-${month}-${day} ${hours}:${minutes}:${seconds}`
    } catch (error) {
        console.warn('Date formatting error:', error)
        return 'N/A'
    }
}

const tableColumns = computed(() => {
    const baseColumns = [
        {
            title: '点位ID',
            dataIndex: 'id',
            width: 120,
            ellipsis: true,
            tooltip: true
        },
        {
            title: '点位名称',
            dataIndex: 'name',
            width: 150,
            ellipsis: true,
            tooltip: true
        },
        {
            title: '读写权限',
            slotName: 'readwrite',
            width: 108,
            ellipsis: true
        },
        {
            title: '数值',
            slotName: 'value',
            width: 150,
            ellipsis: true
        },
        {
            title: '质量',
            slotName: 'quality',
            width: 140,
            ellipsis: true
        },
        {
            title: '采集时间',
            slotName: 'timestamp',
            width: 170,
            ellipsis: true
        },
        {
            title: '操作',
            slotName: 'actions',
            width: 260
        }
    ]
    
    // Add Modbus-specific columns for Modbus protocols
    if (channelProtocol.value.startsWith('modbus')) {
        return [
            baseColumns[0],
            baseColumns[1],
            {
                title: '寄存器地址',
                dataIndex: 'address',
                width: 120,
                ellipsis: true,
                tooltip: true,
                customRender: ({ text }) => text || 'N/A'
            },
            {
                title: '寄存器类型',
                dataIndex: 'register_type',
                width: 180,
                ellipsis: true,
                tooltip: true,
                customRender: ({ text }) => {
                    if (!text) return 'N/A'
                    const lowerText = text.toLowerCase()
                    if (lowerText.includes('holding')) return 'HOLDING_REGISTER (保持寄存器)'
                    if (lowerText.includes('input')) return 'INPUT_REGISTER (输入寄存器)'
                    if (lowerText.includes('coil')) return 'COIL (输出线圈)'
                    if (lowerText.includes('discrete')) return 'DISCRETE_INPUT (离散输入)'
                    return text
                }
            },
            ...baseColumns.slice(2)
        ]
    }
    
    // For other protocols, add a generic address column
    return [
        baseColumns[0],
        baseColumns[1],
        {
            title: getProtocolAddressLabel(),
            dataIndex: 'address',
            width: 200,
            ellipsis: true,
            tooltip: true,
            customRender: ({ text }) => text || 'N/A'
        },
        ...baseColumns.slice(2)
    ]
})

const cloneTableColumns = [
    {
        title: '选择',
        slotName: 'checkbox',
        width: 80,
        fixed: 'left'
    },
    {
        title: '点位ID',
        dataIndex: 'id',
        width: 120,
        ellipsis: true,
        tooltip: true
    },
    {
        title: '点位名称',
        dataIndex: 'name',
        width: 150,
        ellipsis: true,
        tooltip: true
    },
    {
        title: '地址',
        dataIndex: 'address',
        width: 150,
        ellipsis: true,
        tooltip: true
    },
    {
        title: '数据类型',
        dataIndex: 'datatype',
        width: 100,
        ellipsis: true
    },
    {
        title: '单位',
        dataIndex: 'unit',
        width: 80,
        ellipsis: true
    },
    {
        title: '读写权限',
        dataIndex: 'readwrite',
        width: 100,
        ellipsis: true
    }
]

const filteredPoints = computed(() => {
    try {
        let result = points.value || []
        
        // Search filter
        if (filters.search) {
            const s = filters.search.toLowerCase()
            result = result.filter(p => {
                if (!p) return false
                const addr = String(p.address ?? '')
                const plc = p.plc_address != null ? String(p.plc_address) : ''
                return (
                    (p.id && typeof p.id === 'string' && p.id.toLowerCase().includes(s)) ||
                    (p.name && typeof p.name === 'string' && p.name.toLowerCase().includes(s)) ||
                    addr.toLowerCase().includes(s) ||
                    plc.includes(s)
                )
            })
        }
        
        // Quality filter
        if (filters.quality && filters.quality.length > 0) {
            result = result.filter(p => {
                if (!p || p.quality === null || p.quality === undefined) return false
                return filters.quality.includes(p.quality)
            })
        }
        
        return result
    } catch (error) {
        console.error('Filtering error:', error)
        return points.value || []
    }
})

const toggleSelectAll = (val) => {
    if (val) {
        selection.selectedIds = filteredPoints.value.map(p => p.id)
    } else {
        selection.selectedIds = []
    }
}

// Prune selection only when raw point IDs disappear — not on search/quality filters.
const pointIdKey = computed(() =>
    JSON.stringify([...new Set(points.value.map(point => point.id))].sort())
)

watch(pointIdKey, () => {
    const existingIds = new Set(points.value.map(point => point.id))
    selection.selectedIds = [...new Set(
        selection.selectedIds.filter(id => existingIds.has(id))
    )]
})

// Watch selectedIds to update selectAll state
watch(() => selection.selectedIds, (newIds) => {
    if (filteredPoints.value.length === 0) {
        selection.selectAll = false
    } else {
        selection.selectAll = newIds.length === filteredPoints.value.length
    }
}, { deep: true })

const confirmBatchDelete = () => {
    if (selection.selectedIds.length === 0) return
    
    deleteDialog.isBatch = true
    deleteDialog.batchCount = selection.selectedIds.length
    deleteDialog.visible = true
}

// Write Dialog State
const writeDialog = reactive({
    visible: false,
    deviceID: '',
    pointID: '',
    dataType: '',
    valueNum: 0,
    valueStr: '',
    valueBool: false,
    valueBinary: '',
    binaryEncoding: 'hex',
    valueDateTime: null,
    valueStatusCode: 'Good',
    priority: 16, // Default priority
    loading: false
})

// Point Dialog State (Add/Edit)
// Load register offset from localStorage if it exists
const loadRegisterOffset = () => {
    try {
        const savedOffset = localStorage.getItem('modbus_register_offset')
        return savedOffset ? parseInt(savedOffset) : 0
    } catch (e) {
        console.error('Error loading register offset from localStorage:', e)
        return 0
    }
}

// Save register offset to localStorage
const saveRegisterOffset = (offset) => {
    try {
        localStorage.setItem('modbus_register_offset', offset.toString())
    } catch (e) {
        console.error('Error saving register offset to localStorage:', e)
    }
}

const pointDialog = reactive({
    visible: false,
    isEdit: false,
    loading: false,
    registerType: 'holding',
    registerIndex: 1,
    functionCode: 3,
    bacnetType: 'AnalogInput',
    bacnetInstance: 1,
    dlt645DeviceAddr: '',
    dlt645DataID: '',
    knx: {
        mainGroup: 1,
        middleGroup: 1,
        subGroup: 1,
        individualAddress: '',
        bitWidth: null
    },
    byteLength: 4,
    wordOrderOption: 'ABCD',
    parseType: 'FLOAT32',
    defaultValue: '',
    registerOffset: loadRegisterOffset(),
    form: {
        id: '',
        name: '',
        address: '',
        format: '',
        datatype: 'float32',
        readwrite: 'R',
        unit: '',
        scale: 1.0,
        offset: 0.0,
        read_formula: '',
        write_formula: '',
        read_formula_template: null,
        write_formula_template: null
    }
})

const datatypeOptions = [
    'int8',
    'uint8',
    'int16',
    'uint16',
    'int32',
    'uint32',
    'int64',
    'uint64',
    'float32',
    'float64',
    'bool',
    'string',
    'bytestring',
    'WORD',
    'DWORD',
    'LWORD'
]

const formatPresets = [
    {
        id: 'Signed',
        label: 'Signed (2 字节 / 1 寄存器)',
        bytes: 2,
        parseType: 'INT16',
        wordOrder: 'AB',
        datatype: 'int16'
    },
    {
        id: 'Unsigned',
        label: 'Unsigned (2 字节 / 1 寄存器)',
        bytes: 2,
        parseType: 'UINT16',
        wordOrder: 'AB',
        datatype: 'uint16'
    },
    {
        id: 'Hex',
        label: 'Hex (2 字节 / 1 寄存器)',
        bytes: 2,
        parseType: 'UINT16',
        wordOrder: 'AB',
        datatype: 'uint16'
    },
    {
        id: 'Binary',
        label: 'Binary (2 字节 / 1 寄存器)',
        bytes: 2,
        parseType: 'UINT16',
        wordOrder: 'AB',
        datatype: 'uint16'
    },
    {
        id: 'LongABCD',
        label: 'LongABCD (4 字节 / 2 寄存器)',
        bytes: 4,
        parseType: 'INT32',
        wordOrder: 'ABCD',
        datatype: 'int32'
    },
    {
        id: 'LongCDAB',
        label: 'LongCDAB (4 字节 / 2 寄存器)',
        bytes: 4,
        parseType: 'INT32',
        wordOrder: 'CDAB',
        datatype: 'int32'
    },
    {
        id: 'LongBADC',
        label: 'LongBADC (4 字节 / 2 寄存器)',
        bytes: 4,
        parseType: 'INT32',
        wordOrder: 'BADC',
        datatype: 'int32'
    },
    {
        id: 'LongDCBA',
        label: 'LongDCBA (4 字节 / 2 寄存器)',
        bytes: 4,
        parseType: 'INT32',
        wordOrder: 'DCBA',
        datatype: 'int32'
    },
    {
        id: 'FloatABCD',
        label: 'FloatABCD (4 字节 / 2 寄存器)',
        bytes: 4,
        parseType: 'FLOAT32',
        wordOrder: 'ABCD',
        datatype: 'float32'
    },
    {
        id: 'FloatCDAB',
        label: 'FloatCDAB (4 字节 / 2 寄存器)',
        bytes: 4,
        parseType: 'FLOAT32',
        wordOrder: 'CDAB',
        datatype: 'float32'
    },
    {
        id: 'FloatBADC',
        label: 'FloatBADC (4 字节 / 2 寄存器)',
        bytes: 4,
        parseType: 'FLOAT32',
        wordOrder: 'BADC',
        datatype: 'float32'
    },
    {
        id: 'FloatDCBA',
        label: 'FloatDCBA (4 字节 / 2 寄存器)',
        bytes: 4,
        parseType: 'FLOAT32',
        wordOrder: 'DCBA',
        datatype: 'float32'
    },
    {
        id: 'DoubleABCDEFGH',
        label: 'DoubleABCDEFGH (8 字节 / 4 寄存器)',
        bytes: 8,
        parseType: 'FLOAT64',
        wordOrder: 'ABCD',
        datatype: 'float64'
    },
    {
        id: 'DoubleGHEFCDAB',
        label: 'DoubleGHEFCDAB (8 字节 / 4 寄存器)',
        bytes: 8,
        parseType: 'FLOAT64',
        wordOrder: 'CDAB',
        datatype: 'float64'
    },
    {
        id: 'DoubleBADCFEHG',
        label: 'DoubleBADCFEHG (8 字节 / 4 寄存器)',
        bytes: 8,
        parseType: 'FLOAT64',
        wordOrder: 'BADC',
        datatype: 'float64'
    },
    {
        id: 'DoubleHGFEDCBA',
        label: 'DoubleHGFEDCBA (8 字节 / 4 寄存器)',
        bytes: 8,
        parseType: 'FLOAT64',
        wordOrder: 'DCBA',
        datatype: 'float64'
    }
]

const formatPresetSelected = ref(null)
const recentFormatIds = ref([])

const wordOrderOptions = baseWordOrderOptions
const parseTypeOptions = baseParseTypeOptions

const wordOrderOptionsForBytes = computed(() => getWordOrderOptionsForBytes(pointDialog.byteLength))

const filteredParseTypes = computed(() => filterParseTypesByBytes(pointDialog.byteLength))

const filteredFormatPresets = computed(() => formatPresets)

const quickValidate = reactive({
    visible: false,
    rawHex: '',
    expected: '',
    previewHtml: '',
    status: '',
    registerValues: '',
    registerBaseAddress: ''
})

const templateDialog = reactive({
    visible: false,
    search: '',
    templates: [...basePointTemplates],
    runtimeTemplates: []
})

const allPointTemplates = computed(() => {
    return [...templateDialog.templates, ...templateDialog.runtimeTemplates]
})

const filteredPointTemplates = computed(() => {
    const proto = channelProtocol.value
    const key = (templateDialog.search || '').trim().toLowerCase()
    return allPointTemplates.value.filter(tpl => {
        if (tpl.protocol && tpl.protocol !== proto) {
            return false
        }
        if (!key) return true
        const text = `${tpl.name || ''} ${tpl.description || ''}`.toLowerCase()
        return text.includes(key)
    })
})

const formulaTemplates = [
    { label: '线性缩放: v * 0.1', expr: 'v * 0.1' },
    { label: '线性缩放: v / 10', expr: 'v / 10' },
    { label: '温度转换: 摄氏转华氏 (v * 1.8 + 32)', expr: 'v * 1.8 + 32' },
    { label: '位运算: 取第0位 (bit0)', expr: 'bitand(v,1) != 0' },
    { label: '位运算: 右移2位 (v >> 2)', expr: 'v >> 2' },
    { label: '高低字交换: 16位 (v >> 8 | (v & 0xFF) << 8)', expr: '(v >> 8) | ((v & 255) << 8)' }
]

const formulaErrors = reactive({
    read: '',
    write: ''
})

const helpDialog = reactive({
    visible: false,
    search: ''
})

const helpSections = [
    {
        id: 'protocol',
        title: '协议解码示例',
        items: [
            {
                title: 'Modbus 电压值 (寄存器值 * 0.1)',
                desc: '寄存器保存的为整数 0.1V 步进的电压值，例如 2301 表示 230.1V，可通过读公式 v * 0.1 得到工程值。',
                snippet: '读公式: v * 0.1'
            },
            {
                title: 'BACnet 多状态量 (右移去掉状态位)',
                desc: '某些多状态点高位为状态标志，低位为实际值，可以通过读公式 v >> 2 提取实际值。',
                snippet: '读公式: v >> 2'
            }
        ]
    },
    {
        id: 'syntax',
        title: '公式语法说明',
        items: [
            {
                title: '基本运算符',
                desc: '支持 +, -, *, /, %, 括号 ()，以及按位运算符 & | ^ << >>。变量名统一为 v。',
                snippet: '示例: (v - 4) * 0.5'
            },
            {
                title: '比较与三元运算',
                desc: '支持 >, >=, <, <=, ==, != 以及条件表达式 condition ? a : b，用于告警或状态映射。',
                snippet: '示例: v > 0 ? 1 : 0'
            }
        ]
    },
    {
        id: 'functions',
        title: '常用函数库',
        items: [
            {
                title: 'bitand / bitor / bitxor',
                desc: '位与 / 位或 / 位异或，用于从状态字中提取和组合标志位。',
                snippet: '示例: bitand(v, 4) != 0'
            },
            {
                title: 'bitnot / bitshl / bitshr',
                desc: '按位取反、左移、右移，对寄存器位进行操作。',
                snippet: '示例: bitshr(v, 1)'
            }
        ]
    },
    {
        id: 'faq',
        title: 'FAQ 与实践建议',
        items: [
            {
                title: '读公式与写公式如何配对',
                desc: '通常读公式是从寄存器值到工程值，写公式则是工程值到寄存器值，例如读: v * 0.1，对应写: v / 0.1。',
                snippet: '读: v * 0.1\n写: v / 0.1'
            },
            {
                title: '公式与缩放比例如何选择',
                desc: '建议优先使用公式描述复杂逻辑，Scale/Offset 只做简单线性换算，避免同一含义重复配置。',
                snippet: ''
            }
        ]
    }
]

const filteredHelpSections = computed(() => {
    const key = (helpDialog.search || '').trim().toLowerCase()
    if (!key) return helpSections
    return helpSections
        .map(section => {
            const items = section.items.filter(item => {
                const text = `${item.title} ${item.desc} ${item.snippet || ''}`.toLowerCase()
                return text.includes(key)
            })
            return { ...section, items }
        })
        .filter(section => section.items.length > 0)
})

const copySnippet = (text) => {
    if (!text) return
    navigator.clipboard.writeText(text)
        .then(() => {
            showMessage('示例已复制到剪贴板', 'success')
        })
        .catch(() => {
            showMessage('复制失败，请手动选择文本', 'warning')
        })
}

const openQuickValidate = () => {
    quickValidate.visible = true
    quickValidate.status = ''
    quickValidate.previewHtml = ''
    quickValidate.rawHex = ''
    quickValidate.registerValues = ''
    quickValidate.expected = ''
    const addr = pointDialog.form.address
    quickValidate.registerBaseAddress = typeof addr === 'string' ? addr : String(addr || '')
}

const runQuickValidate = () => {
    let hex = (quickValidate.rawHex || '').replace(/[^0-9a-fA-F]/g, '')
    if (!hex) {
        const text = (quickValidate.registerValues || '').trim()
        if (text) {
            const parts = text.split(/[\s,]+/).filter(Boolean)
            const regs = parts.map(p => {
                if (p.toLowerCase().startsWith('0x')) {
                    return parseInt(p, 16)
                }
                return parseInt(p, 10)
            })
            const bytes = registersToBytes(regs)
            hex = bytes.map(b => b.toString(16).padStart(2, '0')).join('')
        }
    }
    if (!hex) {
        quickValidate.previewHtml = sanitizeHtml('请输入有效的十六进制报文')
        quickValidate.status = ''
        return
    }
    const bytesNeeded = pointDialog.byteLength || 0
    if (bytesNeeded > 0 && hex.length < bytesNeeded * 2) {
        quickValidate.previewHtml = sanitizeHtml('报文长度不足，无法解析')
        quickValidate.status = ''
        return
    }
    const buf = []
    for (let i = 0; i < hex.length; i += 2) {
        const b = parseInt(hex.slice(i, i + 2), 16)
        if (isNaN(b)) continue
        buf.push(b)
    }
    const slice = bytesNeeded > 0 ? buf.slice(0, bytesNeeded) : buf
    const reordered = reorderBytes(slice, pointDialog.byteLength, pointDialog.wordOrderOption)
    const rawValue = parseByType(reordered, pointDialog.parseType)
    const engineValue = applyFormula(rawValue, pointDialog.form.read_formula, pointDialog.form.scale, pointDialog.form.offset)
    const valueStr = engineValue === undefined ? '解析失败' : String(engineValue)
    const expectedInput = (quickValidate.expected || '').trim()
    let expected = expectedInput
    if (expectedInput) {
        const parts = expectedInput.split(/\s+/)
        expected = parts[parts.length - 1]
    }
    if (expected) {
        const actualNum = Number(engineValue)
        const expectedNum = Number(expected)
        let pass = false
        if (Number.isFinite(actualNum) && Number.isFinite(expectedNum)) {
            pass = Math.abs(actualNum - expectedNum) < 1e-6
        } else {
            pass = String(engineValue) === expected
        }
        quickValidate.status = pass ? 'pass' : 'fail'
        const diffHtml = highlightDiff(valueStr, expected)
        const html = `实际: ${valueStr}<br/>期望: ${expected}<br/>差异: ${diffHtml}`
        quickValidate.previewHtml = sanitizeHtml(html)
    } else {
        quickValidate.status = ''
        quickValidate.previewHtml = sanitizeHtml(valueStr)
    }
}

const highlightDiff = (actual, expected) => {
    const a = String(actual)
    const b = String(expected)
    const len = Math.max(a.length, b.length)
    let html = ''
    for (let i = 0; i < len; i++) {
        const ca = a[i] || ''
        const cb = b[i] || ''
        if (ca === cb) {
            html += ca
        } else {
            html += `<span style="color: red; font-weight: 600">${ca || ' '}</span>`
        }
    }
    return html
}

const updateRecentFormats = (id) => {
    if (!id) {
        return
    }
    const list = recentFormatIds.value.filter(x => x !== id)
    list.unshift(id)
    recentFormatIds.value = list.slice(0, 2)
}

const presetIdToFormat = (id) => {
    if (!id) return ''
    if (id === 'Hex') return 'hex'
    if (id === 'Binary') return 'binary'
    return id
}

const inferPresetFromPoint = (p) => {
    if (!p) return null
    const dt = (p.datatype || '').toLowerCase()
    const fmt = (p.format || '').toLowerCase()
    const wo = (p.word_order || '').toUpperCase()

    if (fmt === 'hex') return 'Hex'
    if (fmt === 'binary') return 'Binary'

    if (dt === 'int16') return 'Signed'
    if (dt === 'uint16') return 'Unsigned'

    if (dt === 'int32') {
        if (wo === 'CDAB') return 'LongCDAB'
        if (wo === 'BADC') return 'LongBADC'
        if (wo === 'DCBA') return 'LongDCBA'
        return 'LongABCD'
    }

    if (dt === 'float32') {
        if (wo === 'CDAB') return 'FloatCDAB'
        if (wo === 'BADC') return 'FloatBADC'
        if (wo === 'DCBA') return 'FloatDCBA'
        return 'FloatABCD'
    }

    if (dt === 'float64') {
        if (wo === 'CDAB') return 'DoubleGHEFCDAB'
        if (wo === 'BADC') return 'DoubleBADCFEHG'
        if (wo === 'DCBA') return 'DoubleHGFEDCBA'
        return 'DoubleABCDEFGH'
    }

    return null
}

const onSelectFormatPreset = (id) => {
    if (!id) {
        return
    }
    const preset = formatPresets.find(p => p.id === id)
    if (!preset) {
        return
    }
    pointDialog.byteLength = preset.bytes
    pointDialog.wordOrderOption = preset.wordOrder
    pointDialog.parseType = preset.parseType
    pointDialog.form.datatype = preset.datatype
    pointDialog.form.format = presetIdToFormat(id)
    updateRecentFormats(id)
}

const toggleRecentFormats = () => {
    if (recentFormatIds.value.length === 0) {
        return
    }
    let target = recentFormatIds.value[0]
    if (formatPresetSelected.value === recentFormatIds.value[0] && recentFormatIds.value[1]) {
        target = recentFormatIds.value[1]
    }
    formatPresetSelected.value = target
    onSelectFormatPreset(target)
}

const openTemplateDialog = () => {
    templateDialog.visible = true
}

const applyTemplate = (tpl) => {
    if (!tpl) return
    const name = tpl.name || ''
    pointDialog.form.name = name
    pointDialog.form.datatype = tpl.datatype || pointDialog.form.datatype
    pointDialog.form.unit = tpl.unit || ''
    pointDialog.form.readwrite = tpl.readwrite || 'R'
    pointDialog.form.read_formula = tpl.readFormula || ''
    pointDialog.form.write_formula = tpl.writeFormula || ''
    pointDialog.byteLength = tpl.byteLength || pointDialog.byteLength
    pointDialog.wordOrderOption = tpl.wordOrder || pointDialog.wordOrderOption
    pointDialog.parseType = tpl.parseType || pointDialog.parseType
    pointDialog.defaultValue = tpl.defaultValue !== undefined ? String(tpl.defaultValue) : ''
    pointDialog.form.word_order = wordOrderToBackend(pointDialog.wordOrderOption)
    validateFormula('read')
    validateFormula('write')
    templateDialog.visible = false
}

const copyTemplate = (tpl) => {
    if (!tpl) return
    const text = JSON.stringify(tpl, null, 2)
    navigator.clipboard.writeText(text)
        .then(() => {
            showMessage('模板配置已复制到剪贴板', 'success')
        })
        .catch(() => {
            showMessage('复制失败，请手动选择文本', 'warning')
        })
}

const saveCurrentAsTemplate = () => {
    if (quickValidate.status !== 'pass') return
    const tpl = {
        id: `custom_${Date.now()}`,
        protocol: channelProtocol.value,
        name: pointDialog.form.name || '自定义模板',
        datatype: pointDialog.form.datatype,
        byteLength: pointDialog.byteLength,
        wordOrder: pointDialog.wordOrderOption,
        parseType: pointDialog.parseType,
        unit: pointDialog.form.unit,
        readFormula: pointDialog.form.read_formula,
        writeFormula: pointDialog.form.write_formula,
        defaultValue: pointDialog.defaultValue,
        readwrite: pointDialog.form.readwrite,
        description: '通过快速验证生成的自定义模板'
    }
    templateDialog.runtimeTemplates.push(tpl)
    showMessage('当前配置已保存为本地模板(会话内有效)', 'success')
}

const validateFormula = (type) => {
    const val = type === 'read' ? (pointDialog.form.read_formula || '') : (pointDialog.form.write_formula || '')
    const target = type === 'read' ? 'read' : 'write'
    if (!val) {
        formulaErrors[target] = ''
        return
    }
    const allowed = /^[0-9vV\+\-\*\/%\&\|\^\<\>\(\)\?\:\s\.]+$/
    if (!allowed.test(val)) {
        formulaErrors[target] = '仅允许使用数字、v、运算符和括号'
        return
    }
    let balance = 0
    for (let i = 0; i < val.length; i++) {
        if (val[i] === '(') balance++
        if (val[i] === ')') balance--
        if (balance < 0) break
    }
    if (balance !== 0) {
        formulaErrors[target] = '括号不匹配'
        return
    }
    formulaErrors[target] = ''
}

const onSelectFormulaTemplate = (type) => {
    if (type === 'read') {
        const tpl = formulaTemplates.find(t => t.expr === pointDialog.form.read_formula_template)
        if (tpl) {
            pointDialog.form.read_formula = tpl.expr
            validateFormula('read')
        }
    } else {
        const tpl = formulaTemplates.find(t => t.expr === pointDialog.form.write_formula_template)
        if (tpl) {
            pointDialog.form.write_formula = tpl.expr
            validateFormula('write')
        }
    }
}

const registerTypes = [
    { label: 'COIL (输出线圈)', value: 'coil' },
    { label: 'DISCRETE_INPUT (离散输入)', value: 'discrete' },
    { label: 'INPUT_REGISTER (输入寄存器)', value: 'input' },
    { label: 'HOLDING_REGISTER (保持寄存器)', value: 'holding' }
]

const registerIndexError = ref('')
const registerOffsetError = ref('')

const getRegisterIndexMin = () => {
    // Get start_address from device config
    const startAddress = deviceInfo.value?.config?.start_address || deviceInfo.value?.config?.address_base || 0
    return startAddress
}

const getRegisterIndexMax = () => {
    // Get start_address from device config
    const startAddress = deviceInfo.value?.config?.start_address || deviceInfo.value?.config?.address_base || 0
    // PDU address max is 65535, so UI display address max is start_address + 65535
    return startAddress + 65535
}

const validateRegisterIndex = () => {
    const idx = parseInt(pointDialog.registerIndex) || 0
    const min = getRegisterIndexMin()
    const max = getRegisterIndexMax()
    
    if (idx < min || idx > max) {
        registerIndexError.value = `寄存器索引必须在 ${min} 到 ${max} 之间`
    } else {
        registerIndexError.value = ''
    }
}

const validateRegisterOffset = () => {
    const offset = parseInt(pointDialog.registerOffset) || 0
    if (offset < 0 || offset > 9999) {
        registerOffsetError.value = '起始偏移量必须在 0 到 9999 之间'
    } else {
        registerOffsetError.value = ''
        saveRegisterOffset(offset)
    }
}

const getProtocolAddressLabel = () => {
    switch (channelProtocol.value) {
        case 's7': return 'S7 地址'
        case 'ethernet-ip': return 'Tag 名称'
        case 'mitsubishi-slmp': return '地址'
        case 'omron-fins': return '地址'
        case 'knxnet-ip': return '组地址'
        case 'profinet-io': return 'IO 地址'
        case 'iec60870-5-104': return 'IOA'
        case 'snmp': return 'OID 地址'
        case 'dlt645': return '数据标识'
        default: return '地址'
    }
}

const getProtocolAddressPlaceholder = () => {
    switch (channelProtocol.value) {
        case 's7': return 'DB1.DBD0 或 M0.0'
        case 'ethernet-ip': return 'Program:Main.MyTag'
        case 'mitsubishi-slmp': return 'D100'
        case 'omron-fins': return 'CIO1.2'
        case 'knxnet-ip': return '1/2/3 或 1/2/3,1.1.1,2'
        case 'profinet-io': return '3:1:0 或 3:2:5.3#LE'
        case 'iec60870-5-104': return '400'
        case 'snmp': return 'public|1.3.6.1.2.1.1.1.0'
        case 'dlt645': return '02-01-01-00'
        default: return '输入地址'
    }
}

const getProtocolAddressTooltip = () => {
    switch (channelProtocol.value) {
        case 's7': return 'DB区: DB1.DBD0(双字), DB1.DBW2(字), DB1.DBX0.1(位), DB1.DBB4(字节) | M区: M0.0(位), MD0(双字), MW0(字) | I区: I0.0(位), ID0(双字) | Q区: Q0.0(位), QD0(双字) | T0(定时器), C0(计数器)'
        case 'ethernet-ip': return '例如: Program:Main.MyTag'
        case 'mitsubishi-slmp': return '格式: D100, M0, X0, D20.2, D100.16L'
        case 'omron-fins': return '格式: CIO1.2, D100, W3.4, EM10.100'
        case 'knxnet-ip': return '格式: main/middle/sub，可选 ",area.line.device,bit"；例如 1/2/3,1.1.1,2'
        case 'profinet-io': return '格式: SLOT:SUB_SLOT:INDEX[.BIT][#ENDIAN]；例如 3:1:0 (int16), 3:2:10 (float), 3:2:5.3 (bit)'
        case 'iec60870-5-104': return 'IOA 0–65535；group 字段填写 TypeID，如 M_ME_NC_1'
        case 'snmp': return 'v2c: community|OID；v3: securityName|OID，如 public|1.3.6.1.2.1.1.1.0'
        case 'dlt645': return '格式: XX-XX-XX-XX'
        default: return ''
    }
}

const updateAddress = () => {
    const idx = parseInt(pointDialog.registerIndex) || 0
    const offset = parseInt(pointDialog.registerOffset) || 0
    let address = 0
    
    // Get start_address from device config
    const startAddress = deviceInfo.value?.config?.start_address || deviceInfo.value?.config?.address_base || 0
    
    // Validate input address
    if (idx < startAddress) {
        registerIndexError.value = `地址不能小于基准地址 ${startAddress}`
        return
    }
    
    // Calculate PDU 0-based address
    address = idx - startAddress + offset
    
    // Validate PDU address range
    if (address < 0 || address > 65535) {
        registerIndexError.value = 'PDU地址必须在 0 到 65535 之间'
        return
    }
    
    registerIndexError.value = ''
    pointDialog.form.address = address.toString()
    
    // Auto set function code based on register type
    const functionCodeMap = {
        'coil': 1,          // COIL (输出线圈)
        'discrete': 2,      // DISCRETE_INPUT (离散输入)
        'input': 4,         // INPUT_REGISTER (输入寄存器)
        'holding': 3        // HOLDING_REGISTER (保持寄存器)
    }
    
    if (functionCodeMap[pointDialog.registerType]) {
        pointDialog.functionCode = functionCodeMap[pointDialog.registerType]
    }
}

const updateBACnetAddress = () => {
	pointDialog.form.address = `${pointDialog.bacnetType}:${pointDialog.bacnetInstance}`
}

const updateDLT645Address = () => {
    if (pointDialog.dlt645DeviceAddr && pointDialog.dlt645DataID) {
        pointDialog.form.address = `${pointDialog.dlt645DeviceAddr}#${pointDialog.dlt645DataID}`
    } else {
        pointDialog.form.address = ''
    }
}

const toKNXInt = (value, min, max) => {
    const num = Number(value)
    if (!Number.isFinite(num)) return null
    return Math.min(max, Math.max(min, Math.trunc(num)))
}

const resetKNXFields = () => {
    pointDialog.knx.mainGroup = 1
    pointDialog.knx.middleGroup = 1
    pointDialog.knx.subGroup = 1
    pointDialog.knx.individualAddress = ''
    pointDialog.knx.bitWidth = null
}

const composeKNXAddress = () => {
    const mainGroup = toKNXInt(pointDialog.knx.mainGroup, 0, 31)
    const middleGroup = toKNXInt(pointDialog.knx.middleGroup, 0, 7)
    const subGroup = toKNXInt(pointDialog.knx.subGroup, 0, 255)

    if (mainGroup === null || middleGroup === null || subGroup === null) {
        return ''
    }

    const parts = [`${mainGroup}/${middleGroup}/${subGroup}`]
    const individualAddress = String(pointDialog.knx.individualAddress || '').trim()
    const bitWidth = toKNXInt(pointDialog.knx.bitWidth, 0, 7)

    if (individualAddress) {
        parts.push(individualAddress)
    }
    if (bitWidth && bitWidth > 0) {
        parts.push(String(bitWidth))
    }

    return parts.join(',')
}

const updateKNXAddress = () => {
    pointDialog.form.address = composeKNXAddress()
}

const parseKNXAddressToUI = (addrStr) => {
    resetKNXFields()

    const raw = String(addrStr || '').trim()
    if (!raw) {
        updateKNXAddress()
        return
    }

    const [groupPart, ...optionalParts] = raw.split(',').map(part => part.trim())
    const groupParts = groupPart.includes('/') ? groupPart.split('/') : groupPart.split('.')
    if (groupParts.length === 3) {
        pointDialog.knx.mainGroup = toKNXInt(groupParts[0], 0, 31) ?? 1
        pointDialog.knx.middleGroup = toKNXInt(groupParts[1], 0, 7) ?? 1
        pointDialog.knx.subGroup = toKNXInt(groupParts[2], 0, 255) ?? 1
    }

    optionalParts.forEach(part => {
        if (!part) return
        if (/^\d+$/.test(part)) {
            pointDialog.knx.bitWidth = toKNXInt(part, 0, 7)
        } else {
            pointDialog.knx.individualAddress = part
        }
    })

    updateKNXAddress()
}

const parseAddressToUI = (addrStr) => {
	if (channelProtocol.value.startsWith('modbus')) {
		const addr = parseInt(addrStr)
		if (isNaN(addr)) return

		// Get start_address from device config
		const startAddress = deviceInfo.value?.config?.start_address || deviceInfo.value?.config?.address_base || 0
		
		// Calculate UI display address
		const displayAddress = addr + startAddress
		
		// Set register type and function code
		pointDialog.registerType = 'holding'
		pointDialog.registerIndex = displayAddress
		pointDialog.functionCode = 3
	} else if (channelProtocol.value === 'bacnet-ip') {
		const parts = addrStr.split(':')
		if (parts.length === 2) {
			pointDialog.bacnetType = parts[0]
			pointDialog.bacnetInstance = parseInt(parts[1]) || 0
		}
	} else if (channelProtocol.value === 'knxnet-ip') {
		parseKNXAddressToUI(addrStr)
	} else if (channelProtocol.value === 'dlt645') {
        const parts = addrStr.split('#')
        if (parts.length === 2) {
            pointDialog.dlt645DeviceAddr = parts[0]
            pointDialog.dlt645DataID = parts[1]
        }
    }
}

watch(
    () => [
        pointDialog.knx.mainGroup,
        pointDialog.knx.middleGroup,
        pointDialog.knx.subGroup,
        pointDialog.knx.individualAddress,
        pointDialog.knx.bitWidth
    ],
    () => {
        if (pointDialog.visible && channelProtocol.value === 'knxnet-ip') {
            updateKNXAddress()
        }
    }
)

// Delete Dialog State
const deleteDialog = reactive({
	visible: false,
	point: null,
	loading: false,
    isBatch: false,
    batchCount: 0
})

const debugDialog = reactive({
    visible: false,
    pointName: '',
    content: ''
})

const registerBlockRegisterTypes = [
    { label: '保持寄存器 (0x03)', value: 'holding' },
    { label: '输入寄存器 (0x04)', value: 'input' },
    { label: '输出线圈 (0x01)', value: 'coil' },
    { label: '离散输入 (0x02)', value: 'discrete' },
]

const registerBlockFunctionCodeMap = {
    coil: 1,
    discrete: 2,
    input: 4,
    holding: 3,
}

const registerBlockDatatypeOptions = [
    { label: 'int16', value: 'int16' },
    { label: 'uint16', value: 'uint16' },
    { label: 'int32', value: 'int32' },
    { label: 'float32', value: 'float32' },
    { label: 'bool', value: 'bool' },
]

const registerBlockReadWriteOptions = [
    { label: '只读 (R)', value: 'R' },
    { label: '读写 (RW)', value: 'RW' },
]

const registerBlockDialog = reactive({
    visible: false,
    loading: false,
    start: 0,
    end: 199,
    datatype: 'int16',
    readwrite: 'R',
    register_type: 'holding',
    function_code: 3,
    mode: 'merge'
})

const registerBlockCount = computed(() => {
    const start = Number(registerBlockDialog.start) || 0
    const end = Number(registerBlockDialog.end) || 0
    if (end < start) return 0
    return end - start + 1
})

const openRegisterBlockDialog = () => {
    registerBlockDialog.start = 0
    registerBlockDialog.end = 199
    registerBlockDialog.datatype = 'int16'
    registerBlockDialog.readwrite = 'R'
    registerBlockDialog.register_type = 'holding'
    registerBlockDialog.function_code = 3
    registerBlockDialog.mode = 'merge'
    registerBlockDialog.visible = true
}

const onRegisterBlockTypeChange = (type) => {
    registerBlockDialog.function_code = registerBlockFunctionCodeMap[type] || 3
    if (type === 'coil' || type === 'discrete') {
        registerBlockDialog.datatype = 'bool'
        registerBlockDialog.readwrite = type === 'coil' ? 'RW' : 'R'
    } else if (registerBlockDialog.datatype === 'bool') {
        registerBlockDialog.datatype = 'int16'
    }
}

const submitRegisterBlock = async () => {
    registerBlockDialog.loading = true
    try {
        const start = Number(registerBlockDialog.start) || 0
        const end = Number(registerBlockDialog.end) || 0
        const res = await request.post(
            channelDeviceApiPath(channelId.value, deviceId.value, 'points', 'generate-registers'),
            {
                start: Math.min(start, end),
                end: Math.max(start, end),
                datatype: registerBlockDialog.datatype,
                readwrite: registerBlockDialog.readwrite,
                register_type: registerBlockDialog.register_type,
                function_code: registerBlockDialog.function_code,
                mode: registerBlockDialog.mode
            }
        )
        Message.success(`已生成 ${res.points_count || registerBlockCount.value} 个点位`)
        registerBlockDialog.visible = false
        await fetchPoints({ force: true })
    } catch (e) {
        Message.error('批量创建失败: ' + (e.message || e))
        throw e
    } finally {
        registerBlockDialog.loading = false
    }
}

const openAddDialog = () => {
	pointDialog.isEdit = false
	pointDialog.form = {
		id: '',
		name: '',
		address: '',
        format: '',
		datatype: 'float32',
		readwrite: 'R',
		unit: '',
		scale: 1.0,
		offset: 0.0,
        read_formula: '',
        write_formula: '',
        read_formula_template: null,
        write_formula_template: null
	}
	
	// Defaults
	pointDialog.registerType = 'holding'
	pointDialog.registerIndex = 1
	pointDialog.functionCode = 3
	pointDialog.bacnetType = 'AnalogInput'
	pointDialog.bacnetInstance = 1
    pointDialog.dlt645DeviceAddr = ''
    pointDialog.dlt645DataID = ''
    resetKNXFields()
    pointDialog.byteLength = 4
    pointDialog.wordOrderOption = 'ABCD'
    pointDialog.parseType = 'FLOAT32'
    pointDialog.defaultValue = ''
	
	if (channelProtocol.value.startsWith('modbus')) {
		// Get start_address from device config
		const startAddress = deviceInfo.value?.config?.start_address || deviceInfo.value?.config?.address_base || 0
		// Set default address based on start_address
		pointDialog.form.address = startAddress.toString()
		// Set register index based on start_address
		pointDialog.registerIndex = startAddress
	} else if (channelProtocol.value === 'bacnet-ip') {
		pointDialog.form.address = 'AnalogInput:1'
	} else if (channelProtocol.value === 'knxnet-ip') {
		updateKNXAddress()
		pointDialog.form.datatype = 'bool'
		pointDialog.byteLength = 1
		pointDialog.parseType = 'UINT8'
	} else if (channelProtocol.value === 'profinet-io') {
		pointDialog.form.address = '3:1:0'
		pointDialog.form.datatype = 'int16'
		pointDialog.byteLength = 2
		pointDialog.parseType = 'INT16'
	} else if (channelProtocol.value === 'dlt645') {
        if (deviceInfo.value && deviceInfo.value.config) {
            const addr = deviceInfo.value.config.station_address || deviceInfo.value.config.address || ''
            if (addr) {
                pointDialog.dlt645DeviceAddr = addr
                // Pre-fill a common data ID or leave empty
                pointDialog.dlt645DataID = '02-01-01-00' // Voltage A
                updateDLT645Address()
            }
        }
    }
	
	pointDialog.visible = true
}

const openEditDialog = (point) => {
    pointDialog.isEdit = true
    resetKNXFields()
    
    // Try to find full config from deviceInfo if available
    if (deviceInfo.value && deviceInfo.value.points) {
        const fullPoint = deviceInfo.value.points.find(p => p.id === point.id)
        if (fullPoint) {
            pointDialog.form = { ...fullPoint }
            if (pointDialog.form.scale === undefined) pointDialog.form.scale = 1.0
            if (pointDialog.form.offset === undefined) pointDialog.form.offset = 0.0
            if (pointDialog.form.read_formula === undefined) pointDialog.form.read_formula = ''
            if (pointDialog.form.write_formula === undefined) pointDialog.form.write_formula = ''
        } else {
            pointDialog.form = {
                ...point,
                scale: 1.0,
                offset: 0.0,
                read_formula: '',
                write_formula: '',
                read_formula_template: null,
                write_formula_template: null
            }
        }
    } else {
        pointDialog.form = {
            ...point,
            scale: 1.0,
            offset: 0.0,
            read_formula: '',
            write_formula: '',
            read_formula_template: null,
            write_formula_template: null
        }
    }
    
    if (pointDialog.form.address) {
        parseAddressToUI(pointDialog.form.address)
    }
    
    // 加载register_type和function_code
    if (pointDialog.form.register_type) {
        pointDialog.registerType = pointDialog.form.register_type
    }
    if (pointDialog.form.function_code) {
        pointDialog.functionCode = pointDialog.form.function_code
    } else {
        // 根据registerType设置默认functionCode
        const typeToCode = { 'coil': 1, 'discrete': 2, 'input': 4, 'holding': 3 }
        pointDialog.functionCode = typeToCode[pointDialog.registerType] || 3
    }
    
    const dt = (pointDialog.form.datatype || '').toLowerCase()
    if (dt === 'int16' || dt === 'uint16' || dt === 'bool' || dt === 'word') {
        pointDialog.byteLength = 2
    } else if (dt === 'int32' || dt === 'uint32' || dt === 'float32' || dt === 'dword' || dt === 'lword') {
        pointDialog.byteLength = 4
    } else if (dt === 'float64') {
        pointDialog.byteLength = 8
    }
    const wo = (pointDialog.form.word_order || '').toUpperCase()
    if (pointDialog.byteLength === 2) {
        if (wo === 'DCBA') {
            pointDialog.wordOrderOption = 'BA'
        } else {
            pointDialog.wordOrderOption = 'AB'
        }
    } else if (wo) {
        pointDialog.wordOrderOption = wo
    }

    const presetId = inferPresetFromPoint(pointDialog.form)
    formatPresetSelected.value = presetId

    pointDialog.visible = true
}

const submitPoint = async () => {
    pointDialog.loading = true
    try {
        if (channelProtocol.value === 'knxnet-ip') {
            updateKNXAddress()
        }

        const url = pointDialog.isEdit 
            ? channelDeviceApiPath(channelId.value, deviceId.value, 'points', pointDialog.form.id)
            : channelDeviceApiPath(channelId.value, deviceId.value, 'points')
        if (formatPresetSelected.value) {
            pointDialog.form.format = presetIdToFormat(formatPresetSelected.value)
        }
        pointDialog.form.word_order = wordOrderToBackend(pointDialog.wordOrderOption)
        
        // 添加寄存器类型和功能码
        pointDialog.form.register_type = pointDialog.registerType
        if (pointDialog.functionCode && pointDialog.functionCode !== 0) {
            pointDialog.form.function_code = pointDialog.functionCode
        }
        
        if (pointDialog.isEdit) {
            const resp = await request.put(url, pointDialog.form)
            const msg = resp?.device_restarted
                ? '点位更新成功，南向设备已重启'
                : '点位更新成功，北向 OPC UA 已同步（未重启网关）'
            showMessage(msg, 'success')
        } else {
            await request.post(url, pointDialog.form)
            showMessage('点位添加成功', 'success')
        }

        pointDialog.visible = false
        fetchPoints() // Refresh list
    } catch (e) {
        showMessage(e.message, 'error')
    } finally {
        pointDialog.loading = false
    }
}

const confirmDelete = (point) => {
    deleteDialog.isBatch = false
    deleteDialog.point = point
    deleteDialog.visible = true
}

const executeDelete = async () => {
    deleteDialog.loading = true
    try {
        if (deleteDialog.isBatch) {
            // Batch delete
            await request.delete(channelDeviceApiPath(channelId.value, deviceId.value, 'points'), { data: selection.selectedIds })
            showMessage(`成功删除 ${selection.selectedIds.length} 个点位`, 'success')
            selection.selectedIds = []
        } else {
            // Single delete
            if (!deleteDialog.point) return
            await request.delete(channelDeviceApiPath(channelId.value, deviceId.value, 'points', deleteDialog.point.id))
            showMessage('点位删除成功', 'success')
        }

        deleteDialog.visible = false
        fetchPoints() // Refresh list
    } catch (e) {
        showMessage(e.message, 'error')
    } finally {
        deleteDialog.loading = false
    }
}

const cloneDialog = reactive({
    visible: false,
    loading: false,
    channels: [],
    selectedChannel: null,
    devices: [],
    selectedDevice: null,
    points: [],
    selected: [],
    selectAll: false,
    search: ''
})

const openCloneDialog = async () => {
    cloneDialog.visible = true
    cloneDialog.loading = true
    cloneDialog.channels = []
    cloneDialog.devices = []
    cloneDialog.points = []
    cloneDialog.selected = []
    cloneDialog.selectAll = false
    try {
        const chs = await request.get('/api/channels', { timeout: 10000, silent: true })
        const same = (chs || []).filter(ch => ch.protocol === channelProtocol.value)
        cloneDialog.channels = same
        if (same.length === 1) {
            cloneDialog.selectedChannel = same[0].id
            await onCloneChannelChange(same[0].id)
        }
    } catch (e) {
    } finally {
        cloneDialog.loading = false
    }
}

const onCloneChannelChange = async (cid) => {
    cloneDialog.loading = true
    cloneDialog.devices = []
    cloneDialog.points = []
    cloneDialog.selected = []
    cloneDialog.selectAll = false
    try {
        if (!cid) return
        const devs = await request.get(`/api/channels/${cid}/devices`, { timeout: 10000, silent: true })
        const list = devs || []
        cloneDialog.devices = list.filter(d => !(cid === channelId.value && d.id === deviceId.value)).map(d => ({
            label: d.name || d.id || 'Unknown Device',
            value: d.id
        }))
    } catch (e) {
    } finally {
        cloneDialog.loading = false
    }
}

const onCloneDeviceChange = async (did) => {
    cloneDialog.loading = true
    cloneDialog.points = []
    cloneDialog.selected = []
    cloneDialog.selectAll = false
    try {
        const cid = cloneDialog.selectedChannel
        if (!cid || !did) return
        const pts = await request.get(channelDeviceApiPath(cid, did, 'points'), { timeout: 8000, silent: true })
        cloneDialog.points = (pts || []).map(p => ({
            id: p.id,
            name: p.name,
            address: p.address,
            datatype: p.datatype,
            unit: p.unit || '',
            readwrite: p.readwrite || 'R'
        }))
    } catch (e) {
    } finally {
        cloneDialog.loading = false
    }
}

const toggleCloneSelectAll = () => {
    if (cloneDialog.selectAll) {
        cloneDialog.selected = [...cloneDialog.points]
    } else {
        cloneDialog.selected = []
    }
}

const filteredClonePoints = computed(() => {
    const list = cloneDialog.points || []
    const key = (cloneDialog.search || '').trim().toLowerCase()
    if (!key) return list
    return list.filter(p => {
        const name = (p.name || '').toLowerCase()
        const addr = (p.address || '').toLowerCase()
        return name.includes(key) || addr.includes(key)
    })
})

const executeClone = async () => {
    if (!cloneDialog.selected || cloneDialog.selected.length === 0) {
        showMessage('请先选择要复制的点位', 'warning')
        return
    }
    cloneDialog.loading = true
    try {
        const payload = cloneDialog.selected.map(p => ({
            id: p.id,
            name: p.name,
            address: p.address,
            datatype: p.datatype,
            unit: p.unit || '',
            readwrite: p.readwrite || 'R',
            scale: 1.0,
            offset: 0.0,
            read_formula: '',
            write_formula: ''
        }))

        await request.post(channelDeviceApiPath(channelId.value, deviceId.value, 'points'), payload, { timeout: 10000, silent: true })
        showMessage(`克隆完成：成功 ${payload.length} 个`, 'success')
        cloneDialog.visible = false
        await fetchPoints()
    } catch (e) {
        showMessage(`克隆失败：${e.message || '未知错误'}`, 'error')
    } finally {
        cloneDialog.loading = false
    }
}

const fetchPoints = async (options = {}) => {
    const { force = false } = options
    if (force) {
        deviceInfo.value = null
    }
    console.log('Fetching points for channel:', channelId.value, 'device:', deviceId.value)
    loading.value = true
    try {
        // 1) 优先获取设备信息（包含点位配置），快速首屏渲染
        if (!deviceInfo.value) {
            try {
                console.log('Fetching device info...')
                const dev = await request.get(channelDeviceApiPath(channelId.value, deviceId.value))
                if (dev) {
                    deviceInfo.value = dev
                    globalState.navTitle = deviceInfo.value.name
                    console.log('Device info fetched:', dev.name, 'Points count:', dev.points?.length)
                }
            } catch (e) {
                console.error('Failed to fetch device info', e)
                showMessage('获取设备信息失败: ' + e.message, 'error')
            }
        }

        // 2) 对于 OPC-UA 设备，确保 endpoint 配置存在
        if (channelProtocol.value === 'opc-ua' && deviceInfo.value) {
            const endpoint = deviceInfo.value.config?.endpoint ||
                channelInfo.value?.config?.url ||
                channelInfo.value?.config?.endpoint
            if (!endpoint || typeof endpoint !== 'string' || endpoint.length === 0) {
                showMessage('OPC UA 未配置 endpoint（请在采集通道填写 Endpoint URL）', 'warning')
            }
        }

        // 3) 用设备配置中的点位生成基础列表；失败时保留已有缓存，不预先清空
        if (deviceInfo.value && Array.isArray(deviceInfo.value.points)) {
            const now = new Date()
            points.value = deviceInfo.value.points.map(p => mapDevicePoint({ ...p, value: null, quality: 'Bad' }, now))
            syncPointIndex()
            loadError.value = ''
            console.log('Initial points list created from device info:', points.value.length)
        } else if (!deviceInfo.value) {
            console.warn('No device info available; keeping cached points if any')
        } else {
            console.warn('No points found in device info')
        }

        // 4) 合并实时缓存（快速填充值，仅当前设备）
        try {
            console.log('Fetching realtime values...')
            const realtime = await request.get(`/api/values/realtime?channel_id=${channelId.value}&device_id=${deviceId.value}`)
            if (realtime && typeof realtime === 'object') {
                console.log('Realtime values fetched:', Object.keys(realtime).length)
                for (let i = 0; i < points.value.length; i++) {
                    const pid = points.value[i].id
                    const v = realtime[pid]
                    if (v) {
                        points.value[i].value = v.value
                        points.value[i].quality = v.quality || 'Good'
                        if (v.collected_at) points.value[i].collected_at = v.collected_at
                        if (v.updated_at) points.value[i].updated_at = v.updated_at
                        if (v.ts) points.value[i].timestamp = v.ts
                        if (v.timestamp) points.value[i].timestamp = v.timestamp
                    }
                }
            }
        } catch (e) {
            // 实时缓存失败不阻塞 UI
            console.warn('Fetch realtime values failed', e)
        }

        // 5) 后台拉取最新实时值（对于 OPC-UA 设备，增加超时时间）
        // 成功则用返回结果覆盖；失败时按是否有缓存区分阻塞/非阻塞错误
        console.log('Triggering background point fetch...')
        const timeout = channelProtocol.value === 'opc-ua' ? 10000 : 2500
        request.get(channelDeviceApiPath(channelId.value, deviceId.value, 'points'), { timeout: timeout, silent: true })
            .then(pts => {
                if (Array.isArray(pts) && pts.length > 0) {
                    console.log('Background point fetch successful, updating points. Points:', pts.length)
                    console.log('First point data:', pts[0])
                    points.value = pts.map(p => mapDevicePoint(p))
                    syncPointIndex()
                    loadError.value = ''
                } else if (Array.isArray(pts)) {
                    console.log('Background fetch returned empty points array')
                    loadError.value = ''
                    if (pts.length === 0 && points.value.length === 0 && channelProtocol.value === 'opc-ua') {
                        showMessage('OPC-UA 设备未发现点位，请先扫描点位', 'info')
                    }
                }
            })
            .catch((err) => {
                console.warn('Background point fetch failed or timed out:', err.message)
                if (points.value.length === 0) {
                    console.log('Fallback: Attempting full point fetch without short timeout...')
                    request.get(channelDeviceApiPath(channelId.value, deviceId.value, 'points'))
                        .then(pts => {
                            if (Array.isArray(pts)) {
                                points.value = pts.map(p => mapDevicePoint(p))
                                syncPointIndex()
                                loadError.value = ''
                                if (pts.length === 0 && channelProtocol.value === 'opc-ua') {
                                    showMessage('OPC-UA 设备未发现点位，请先扫描点位', 'info')
                                }
                            }
                        })
                        .catch((fallbackErr) => {
                            console.error('Fallback point fetch failed:', fallbackErr.message)
                            loadError.value = '获取点位失败：' + fallbackErr.message
                            if (channelProtocol.value === 'opc-ua') {
                                showMessage('获取 OPC-UA 点位失败，请检查设备连接和配置', 'error')
                            }
                        })
                } else {
                    loadError.value = '获取点位失败：' + err.message
                }
            })
    } catch (e) {
        console.error('fetchPoints error:', e)
        loadError.value = '获取点位失败：' + e.message
        showMessage('获取点位失败: ' + e.message, 'error')
    } finally {
        // 不等待后台拉取完成，首屏已就绪
        loading.value = false
        console.log('fetchPoints finished. Loading state:', loading.value)
    }
}

const scanDialogVisible = ref(false)

const existingAddresses = computed(() => {
    const set = new Set()
    for (const p of (points.value || [])) {
        if (p && p.address) set.add(p.address)
    }
    return set
})



const openDiscoverDialog = () => {
    if (channelProtocol.value === 'opc-ua') {
        // 使用 OPC-UA 专用扫描组件
        opcuaScannerVisible.value = true
    } else {
        // 使用新的扫描对话框组件
        scanDialogVisible.value = true
    }
}

const handleRefreshPoints = () => {
    fetchPoints()
}

// Value Detail Dialog Logic
const valueDialog = reactive({
    visible: false,
    value: '',
    decodedValue: '',
    decodeType: 'text',
    isBase64: false,
    byteLength: 2,
    wordOrder: 'AB'
})

const valueFormatColumns = [
    {
        title: '格式',
        dataIndex: 'format',
        key: 'format',
        width: 140
    },
    {
        title: '值',
        dataIndex: 'value',
        key: 'value',
        render: (_, record) => {
            return h('span', { style: 'font-family: monospace;' }, record.value)
        }
    }
]

const valueFormatData = computed(() => {
    if (!numericFormats.value) return []
    return [
        { format: '有符号整型', value: numericFormats.value.signed, key: 'signed' },
        { format: '无符号整型', value: numericFormats.value.unsigned, key: 'unsigned' },
        { format: '十六进制', value: numericFormats.value.hex, key: 'hex' },
        { format: '二进制', value: numericFormats.value.binary, key: 'binary' }
    ]
})

const valueWordOrderOptions = computed(() => {
    const bytes = Number(valueDialog.byteLength) || 0
    return getWordOrderOptionsForBytes(bytes)
})

const numericFormats = computed(() => {
    const raw = valueDialog.value
    if (raw === '' || raw === null || raw === undefined) return null
    const n = Number(raw)
    if (!Number.isFinite(n)) return null
    const byteLength = Number(valueDialog.byteLength) || 2
    const bits = BigInt(byteLength * 8)
    try {
        let base = BigInt(Math.trunc(n))
        const one = 1n
        const mask = (one << bits) - one
        base = base & mask

        const bytes = new Array(byteLength)
        let tmp = base
        for (let i = byteLength - 1; i >= 0; i--) {
            bytes[i] = Number(tmp & 0xFFn)
            tmp >>= 8n
        }

        let reordered = bytes
        const wo = valueDialog.wordOrder || ''
        if (byteLength > 1 && wo) {
            reordered = reorderBytes(bytes, byteLength, wo)
        }

        let unsigned = 0n
        for (const b of reordered) {
            unsigned = (unsigned << 8n) + BigInt(b & 0xFF)
        }

        const signBit = 1n << (bits - 1n)
        let signed = unsigned
        if (unsigned & signBit) {
            signed = unsigned - (one << bits)
        }

        const hexDigits = Math.max(2, (byteLength * 8) / 4)
        const hex = '0x' + unsigned.toString(16).toUpperCase().padStart(hexDigits, '0')
        const binary = unsigned.toString(2).padStart(byteLength * 8, '0')

        return {
            signed: signed.toString(),
            unsigned: unsigned.toString(),
            hex,
            binary
        }
    } catch (e) {
        return null
    }
})

const isBase64 = (str) => {
    if (typeof str !== 'string' || str.length === 0) return false;
    try {
        return btoa(atob(str)) === str;
    } catch (err) {
        return false;
    }
}

const showFullValue = (payload) => {
    let val = payload
    let byteLength = valueDialog.byteLength
    let wordOrder = valueDialog.wordOrder

    if (payload && typeof payload === 'object' && 'value' in payload) {
        val = payload.value
        if (payload.byteLength) {
            byteLength = payload.byteLength
        }
        if (payload.wordOrder) {
            wordOrder = payload.wordOrder
        }
    }

    if (typeof val === 'object' && val !== null) {
        val = JSON.stringify(val)
    }

    valueDialog.value = String(val)
    valueDialog.decodedValue = ''
    valueDialog.decodeType = 'text'
    valueDialog.byteLength = byteLength || 2
    valueDialog.wordOrder = valueDialog.byteLength > 1 ? (wordOrder || 'AB') : ''

    valueDialog.isBase64 = isBase64(valueDialog.value)
    if (valueDialog.isBase64) {
        tryDecode('text')
    }
    valueDialog.visible = true
}

const tryDecode = (type) => {
    valueDialog.decodeType = type
    if (!valueDialog.value) return
    
    try {
        const raw = atob(valueDialog.value)
        if (type === 'text') {
            const bytes = Uint8Array.from(raw, c => c.charCodeAt(0))
            valueDialog.decodedValue = new TextDecoder().decode(bytes)
        } else if (type === 'hex') {
            let result = ''
            for (let i = 0; i < raw.length; i++) {
                const hex = raw.charCodeAt(i).toString(16)
                result += (hex.length === 2 ? hex : '0' + hex) + ' '
            }
            valueDialog.decodedValue = result.toUpperCase().trim()
        } else if (type === 'json') {
            const bytes = Uint8Array.from(raw, c => c.charCodeAt(0))
            const str = new TextDecoder().decode(bytes)
            valueDialog.decodedValue = JSON.stringify(JSON.parse(str), null, 2)
        }
    } catch (e) {
        valueDialog.decodedValue = 'Decode failed: ' + e.message
    }
}









// WebSocket Logic
let ws = null
const pointIndexById = new Map()

const syncPointIndex = () => {
    pointIndexById.clear()
    const list = points.value || []
    for (let i = 0; i < list.length; i++) {
        pointIndexById.set(list[i].id, i)
    }
}

const connectWs = () => {
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
    const host = window.location.host
    ws = new WebSocket(`${protocol}//${host}/api/ws/values`)

    ws.onopen = () => {
        globalState.wsStatus.connected = true
    }

    ws.onmessage = (event) => {
        try {
            const data = JSON.parse(event.data)
            if (data.channel_id === channelId.value && data.device_id === deviceId.value) {
                const idx = pointIndexById.get(data.point_id)
                if (idx !== undefined) {
                    points.value[idx].value = data.value
                    points.value[idx].quality = data.quality
                    if (data.collected_at) points.value[idx].collected_at = data.collected_at
                    if (data.updated_at) points.value[idx].updated_at = data.updated_at
                    points.value[idx].timestamp = data.collected_at || data.timestamp
                }
            }
        } catch (e) { console.error(e) }
    }

    ws.onclose = () => {
        globalState.wsStatus.connected = false
    }
}

const channelProtocol = ref('')
const channelInfo = ref(null)
const bacnetObjectTypes = [
    'AnalogInput', 'AnalogOutput', 'AnalogValue',
    'BinaryInput', 'BinaryOutput', 'BinaryValue',
    'MultiStateInput', 'MultiStateOutput', 'MultiStateValue'
]

const fetchChannel = async () => {
    try {
        console.log('Fetching channel info for channelId:', channelId.value)
        const data = await request.get(`/api/channels/${channelId.value}`)
        console.log('Channel info data:', data)
        if (data && data.protocol) {
            channelProtocol.value = data.protocol
            channelInfo.value = data
            console.log('Channel protocol set to:', channelProtocol.value)
        }
    } catch (e) {
        console.error('Failed to fetch channel info', e)
    }
}

const metrics = reactive({
    connectionSeconds: 0,
    reconnectCount: 0,
    localAddr: '',
    remoteAddr: '',
    lastDisconnectTime: null,
    loading: false
})

const fetchMetrics = async () => {
    if (!channelId.value) return
    metrics.loading = true
    try {
        const data = await request.get(`/api/channels/${channelId.value}/metrics`)
        if (data) {
            metrics.connectionSeconds = data.connectionSeconds || 0
            metrics.reconnectCount = data.reconnectCount || 0
            metrics.localAddr = data.localAddr || ''
            metrics.remoteAddr = data.remoteAddr || ''
            metrics.lastDisconnectTime = data.lastDisconnectTime || null
        }
    } catch (e) {
        console.error('Failed to fetch metrics', e)
    } finally {
        metrics.loading = false
    }
}

let metricsTimer = null

onMounted(async () => {
    await fetchChannel()
    fetchPoints()
    connectWs()
    fetchMetrics()
    metricsTimer = setInterval(fetchMetrics, 5000)
})

onUnmounted(() => {
    if (ws) ws.close()
    if (metricsTimer) clearInterval(metricsTimer)
})

// Helpers
const formatDuration = (seconds) => {
    if (!seconds || seconds < 0) return '未连接'
    const h = Math.floor(seconds / 3600)
    const m = Math.floor((seconds % 3600) / 60)
    const s = Math.floor(seconds % 60)
    if (h > 0) return `${h}时${m}分${s}秒`
    if (m > 0) return `${m}分${s}秒`
    return `${s}秒`
}


const isQualityGood = (q) => q === 'Good' || q === 'good'

const getRegisterCountForDatatype = (dt) => {
    const t = (dt || '').toLowerCase()
    if (['int32', 'uint32', 'float32', 'dword'].includes(t)) return 2
    if (['int64', 'uint64', 'float64', 'double', 'lword'].includes(t)) return 4
    return 1
}

const getRegisterHint = (point) => {
    const dt = point.datatype || point.dataType || ''
    const count = getRegisterCountForDatatype(dt)
    const addr = typeof point.address === 'string' ? point.address : String(point.address || '')
    const base = Number(addr)
    if (!Number.isFinite(base) || count <= 0) {
        if (dt && addr) return `${addr} · ${dt}`
        if (addr) return addr
        return dt
    }
    if (count === 1) {
        return `${base} (1 reg) · ${dt}`
    }
    const end = base + count - 1
    return `${base}-${end} (${count} regs) · ${dt}`
}

// Write Logic
const openWriteDialog = (point) => {
    writeDialog.deviceID = deviceId.value
    writeDialog.pointID = point.id
    writeDialog.dataType = (point.datatype || '').toLowerCase()
    writeDialog.binaryEncoding = 'hex'
    writeDialog.valueBinary = ''
    writeDialog.valueDateTime = null
    writeDialog.valueStatusCode = 'Good'

    if (isBoolType(writeDialog.dataType)) {
        writeDialog.valueBool = false
    } else if (isStringType(writeDialog.dataType) || isGuidType(writeDialog.dataType)
             || isQualifiedNameType(writeDialog.dataType) || isLocalizedTextType(writeDialog.dataType)
             || isNodeIdType(writeDialog.dataType)) {
        writeDialog.valueStr = ''
    } else if (isBinaryType(writeDialog.dataType)) {
        writeDialog.valueBinary = ''
    } else if (isDateTimeType(writeDialog.dataType)) {
        writeDialog.valueDateTime = new Date()
    } else if (isStatusCodeType(writeDialog.dataType)) {
        writeDialog.valueStatusCode = 'Good'
    } else {
        // 数值类型 (Int, Float, Double) 使用 valueStr 存储以支持科学计数法
        writeDialog.valueStr = ''
        writeDialog.valueNum = 0
    }
    writeDialog.visible = true
}

const submitWrite = async () => {
    if (writeDialog.loading) return
    writeDialog.loading = true
    const closeLoading = Message.loading({
        content: '下发指令中...',
        duration: 0 // 手动关闭
    })

    try {
        const payloadValue = normalizeWriteValue()

        let finalValue = payloadValue
        if (channelProtocol.value === 'bacnet-ip') {
            finalValue = {
                value: payloadValue,
                priority: writeDialog.priority
            }
        }

        await request.post('/api/write', {
            channel_id: channelId.value,
            device_id: deviceId.value,
            point_id: writeDialog.pointID,
            value: finalValue
        })

        closeLoading.close()

        Message.success({
            content: `成功：点位 [${writeDialog.pointID}] 已更新`,
            duration: 2000,
            closable: true
        })

        writeDialog.visible = false
        fetchPoints()
    } catch (e) {
        closeLoading.close()
        Message.error({
            content: `错误：${e.message}`,
            duration: 5000,
            closable: true
        })
    } finally {
        writeDialog.loading = false
    }
}

// 打开点位调试（调用后端 /api/points/:id/debug 并用浏览器提示显示）
const handlePointsAdded = (result) => {
    showMessage(`已添加 ${result.success} 个点位${result.fail > 0 ? `，${result.fail} 个失败` : ''}`, result.fail > 0 ? 'warning' : 'success')
    fetchPoints()
}

const openDebug = async (point) => {
    try {
        const resp = await request.get(`/api/points/${point.id}/debug`, { timeout: 3000, silent: true })
        debugDialog.pointName = point.name || point.id
        debugDialog.content = JSON.stringify(resp, null, 2)
        debugDialog.visible = true
    } catch (e) {
        showMessage('获取点位调试信息失败: ' + (e.message || e), 'error')
    }
}

// OPC-UA 数据类型判断
const isBoolType = (dt) => ['bool', 'boolean', 'bit', 'i=1'].includes((dt || '').toLowerCase())
const isStringType = (dt) => ['string', 'i=12', 'xmlliteral', 'i=16'].includes((dt || '').toLowerCase())
const isBinaryType = (dt) => ['bytestring', 'i=15', 'extensionobject', 'i=22'].includes((dt || '').toLowerCase())
const isFloatType = (dt) => {
    const lowerDt = (dt || '').toLowerCase()
    // 直接匹配
    if (['float', 'float32', 'float64', 'double', 'i=10', 'i=11'].includes(lowerDt)) {
        return true
    }
    // 支持带命名空间前缀的 OPC-UA 类型格式 (ns=X;i=10)
    if (/ns=\d+;i=10/.test(dt) || /ns=\d+;i=11/.test(dt)) {
        return true
    }
    return false
}
const isIntType = (dt) => ['int8', 'int16', 'int32', 'int64', 'uint8', 'uint16', 'uint32', 'uint64',
    'sbyte', 'byte', 'i=2', 'i=3', 'i=4', 'i=5', 'i=6', 'i=7', 'i=8', 'i=9',
    'word', 'dword', 'lword', 'int', 'uint'].includes((dt || '').toLowerCase())

// OPC-UA 扩展类型判断
const isDateTimeType = (dt) => ['datetime', 'i=13', 'date'].includes((dt || '').toLowerCase())
const isGuidType = (dt) => ['guid', 'i=14'].includes((dt || '').toLowerCase())
const isNodeIdType = (dt) => ['nodeid', 'i=17'].includes((dt || '').toLowerCase())
const isStatusCodeType = (dt) => ['statuscode', 'i=19'].includes((dt || '').toLowerCase())
const isQualifiedNameType = (dt) => ['qualifiedname', 'i=20'].includes((dt || '').toLowerCase())
const isLocalizedTextType = (dt) => ['localizedtext', 'i=21'].includes((dt || '').toLowerCase())

// Ethernet/IP (enip) 数据类型判断 - 基于 Logix 控制器类型
const isEnipRealType = (dt) => ['real', 'float32'].includes((dt || '').toLowerCase())
const isEnipLrealType = (dt) => ['lreal', 'float64', 'double'].includes((dt || '').toLowerCase())
const isEnipBoolType = (dt) => ['bool'].includes((dt || '').toLowerCase())
const isEnipSintType = (dt) => ['sint', 'int8', 'int8s'].includes((dt || '').toLowerCase())
const isEnipIntType = (dt) => ['int', 'int16', 'int16s'].includes((dt || '').toLowerCase())
const isEnipDintType = (dt) => ['dint', 'int32', 'int32s'].includes((dt || '').toLowerCase())
const isEnipLintType = (dt) => ['lint', 'int64', 'int64s'].includes((dt || '').toLowerCase())
const isEnipUsintType = (dt) => ['usint', 'uint8', 'uint8u'].includes((dt || '').toLowerCase())
const isEnipUintType = (dt) => ['uint', 'uint16', 'uint16u'].includes((dt || '').toLowerCase())
const isEnipUdintType = (dt) => ['udint', 'uint32', 'uint32u'].includes((dt || '').toLowerCase())
const isEnipUlintType = (dt) => ['ulint', 'uint64', 'uint64u'].includes((dt || '').toLowerCase())
const isEnipStringType = (dt) => ['string'].includes((dt || '').toLowerCase())
const isEnipFloatType = (dt) => isEnipRealType(dt) || isEnipLrealType(dt)
const isEnipNumericType = (dt) => isEnipSintType(dt) || isEnipIntType(dt) || isEnipDintType(dt) || isEnipLintType(dt) ||
                                isEnipUsintType(dt) || isEnipUintType(dt) || isEnipUdintType(dt) || isEnipUlintType(dt) ||
                                isEnipRealType(dt) || isEnipLrealType(dt)

// OPC-UA 状态码选项
const statusCodeOptions = [
    { label: 'Good (成功)', value: 'Good' },
    { label: 'Bad (失败)', value: 'Bad' },
    { label: 'Uncertain (不确定)', value: 'Uncertain' },
    { label: 'GoodClamped (被限制)', value: 'GoodClamped' },
    { label: 'GoodLocalOverride (本地覆盖)', value: 'GoodLocalOverride' },
    { label: 'BadNoCommunication (无通信)', value: 'BadNoCommunication' },
    { label: 'BadNotConnected (未连接)', value: 'BadNotConnected' },
    { label: 'BadTimeout (超时)', value: 'BadTimeout' },
]

const getBinaryWritePlaceholder = () => {
    return writeDialog.binaryEncoding === 'base64'
        ? '请输入 Base64，例如 MTIz'
        : '请输入 Hex，例如 0x313233'
}

const getBinaryWriteHint = () => {
    return writeDialog.binaryEncoding === 'base64'
        ? '示例: MTIz'
        : '示例: 0x313233'
}

// 获取数据类型标签
const getDataTypeLabel = () => {
    const dt = (writeDialog.dataType || '').toLowerCase()
    const typeLabels = {
        'bool': 'Boolean', 'boolean': 'Boolean', 'bit': 'Boolean', 'i=1': 'Boolean (i=1)',
        'string': 'String', 'i=12': 'String (i=12)', 'xmlliteral': 'XMLLiteral', 'i=16': 'String (i=16)',
        'bytestring': 'ByteString', 'i=15': 'ByteString (i=15)', 'extensionobject': 'ExtensionObject', 'i=22': 'ExtensionObject',
        'datetime': 'DateTime', 'i=13': 'DateTime (i=13)', 'date': 'Date',
        'guid': 'Guid', 'i=14': 'Guid (i=14)',
        'float': 'Float', 'float32': 'Float32', 'i=10': 'Float (i=10)',
        'float64': 'Float64', 'double': 'Double', 'i=11': 'Double (i=11)',
        'int8': 'Int8', 'int16': 'Int16', 'int32': 'Int32', 'int64': 'Int64',
        'uint8': 'UInt8', 'uint16': 'UInt16', 'uint32': 'UInt32', 'uint64': 'UInt64',
        'sbyte': 'SByte', 'byte': 'Byte', 'i=2': 'SByte (i=2)', 'i=3': 'Byte (i=3)',
        'i=4': 'Int16 (i=4)', 'i=5': 'UInt16 (i=5)', 'i=6': 'Int32 (i=6)', 'i=7': 'UInt32 (i=7)',
        'i=8': 'Int64 (i=8)', 'i=9': 'UInt64 (i=9)',
        'word': 'Word', 'dword': 'DWord', 'lword': 'LWord',
        'int': 'Int', 'uint': 'UInt',
        'nodeid': 'NodeId', 'i=17': 'NodeId (i=17)',
        'statuscode': 'StatusCode', 'i=19': 'StatusCode (i=19)',
        'qualifiedname': 'QualifiedName', 'i=20': 'QualifiedName (i=20)',
        'localizedtext': 'LocalizedText', 'i=21': 'LocalizedText (i=21)'
    }
    return typeLabels[dt] || dt || 'Unknown'
}

// 获取字符串输入提示
const getStringInputPlaceholder = () => {
    const dt = (writeDialog.dataType || '').toLowerCase()
    if (isQualifiedNameType(dt)) return '命名空间:名称 (如 2:Temperature)'
    if (isLocalizedTextType(dt)) return '请输入文本'
    return '请输入字符串'
}

// 获取数值类型信息
const getNumericTypeInfo = () => {
    const dt = (writeDialog.dataType || '').toLowerCase()
    if (isFloatType(dt)) {
        if (dt === 'float' || dt === 'float32' || dt === 'i=10') return 'FLOAT32 (32位单精度)'
        if (dt === 'float64' || dt === 'double' || dt === 'i=11') return 'FLOAT64 (64位双精度)'
    }
    if (isIntType(dt)) {
        if (['int8', 'uint8', 'byte', 'sbyte'].includes(dt) || /i=[2-3]/.test(dt)) return 'INT8/UINT8 (8位整数)'
        if (['int16', 'uint16', 'word'].includes(dt) || dt === 'i=4' || dt === 'i=5') return 'INT16/UINT16 (16位整数)'
        if (['int32', 'uint32', 'dword'].includes(dt) || dt === 'i=6' || dt === 'i=7') return 'INT32/UINT32 (32位整数)'
        if (['int64', 'uint64', 'lword'].includes(dt) || dt === 'i=8' || dt === 'i=9') return 'INT64/UINT64 (64位整数)'
    }
    return ''
}

const normalizeWriteValue = () => {
    const dt = (writeDialog.dataType || '').toLowerCase()
    const proto = channelProtocol.value || ''

    // ==================== Ethernet/IP (enip) 协议特殊处理 ====================
    // 对于 enip-simulator 设备，需要按照 Logix 控制器类型进行严格的类型处理
    if (proto === 'ethernet-ip') {
        // BOOL 类型
        if (isEnipBoolType(dt)) {
            return writeDialog.valueBool
        }

        // STRING 类型 - 只有字符串类型接受字符串值
        if (isEnipStringType(dt)) {
            return writeDialog.valueStr || ''
        }

        // REAL/FLOAT32 类型 - 必须使用浮点类型
        if (isEnipRealType(dt)) {
            const inputVal = writeDialog.valueStr !== undefined ? writeDialog.valueStr : writeDialog.valueNum
            const n = parseFloat(inputVal)
            if (!isNaN(n) && isFinite(n)) {
                return n  // 返回 float32 类型的数值
            }
            return 0.0
        }

        // LREAL/FLOAT64/DOUBLE 类型 - 必须使用浮点类型
        if (isEnipLrealType(dt)) {
            const inputVal = writeDialog.valueStr !== undefined ? writeDialog.valueStr : writeDialog.valueNum
            const n = parseFloat(inputVal)
            if (!isNaN(n) && isFinite(n)) {
                return n  // 返回 float64 类型的数值
            }
            return 0.0
        }

        // SINT/INT8 类型
        if (isEnipSintType(dt)) {
            const inputVal = writeDialog.valueStr !== undefined ? writeDialog.valueStr : writeDialog.valueNum
            const n = parseInt(inputVal)
            if (!isNaN(n)) {
                // 限制范围 -128 到 127
                return Math.max(-128, Math.min(127, n))
            }
            return 0
        }

        // INT/INT16 类型
        if (isEnipIntType(dt)) {
            const inputVal = writeDialog.valueStr !== undefined ? writeDialog.valueStr : writeDialog.valueNum
            const n = parseInt(inputVal)
            if (!isNaN(n)) {
                // 限制范围 -32768 到 32767
                return Math.max(-32768, Math.min(32767, n))
            }
            return 0
        }

        // DINT/INT32 类型
        if (isEnipDintType(dt)) {
            const inputVal = writeDialog.valueStr !== undefined ? writeDialog.valueStr : writeDialog.valueNum
            const n = parseInt(inputVal)
            if (!isNaN(n)) {
                // 限制范围 -2147483648 到 2147483647
                return Math.max(-2147483648, Math.min(2147483647, n))
            }
            return 0
        }

        // LINT/INT64 类型
        if (isEnipLintType(dt)) {
            const inputVal = writeDialog.valueStr !== undefined ? writeDialog.valueStr : writeDialog.valueNum
            const n = parseInt(inputVal)
            return isNaN(n) ? 0 : n
        }

        // USINT/UINT8 类型
        if (isEnipUsintType(dt)) {
            const inputVal = writeDialog.valueStr !== undefined ? writeDialog.valueStr : writeDialog.valueNum
            const n = parseInt(inputVal)
            if (!isNaN(n)) {
                // 限制范围 0 到 255
                return Math.max(0, Math.min(255, n))
            }
            return 0
        }

        // UINT/UINT16 类型
        if (isEnipUintType(dt)) {
            const inputVal = writeDialog.valueStr !== undefined ? writeDialog.valueStr : writeDialog.valueNum
            const n = parseInt(inputVal)
            if (!isNaN(n)) {
                // 限制范围 0 到 65535
                return Math.max(0, Math.min(65535, n))
            }
            return 0
        }

        // UDINT/UINT32 类型
        if (isEnipUdintType(dt)) {
            const inputVal = writeDialog.valueStr !== undefined ? writeDialog.valueStr : writeDialog.valueNum
            const n = parseInt(inputVal)
            if (!isNaN(n)) {
                // 限制范围 0 到 4294967295
                return Math.max(0, Math.min(4294967295, n))
            }
            return 0
        }

        // ULINT/UINT64 类型
        if (isEnipUlintType(dt)) {
            const inputVal = writeDialog.valueStr !== undefined ? writeDialog.valueStr : writeDialog.valueNum
            const n = parseInt(inputVal)
            if (!isNaN(n) && n >= 0) {
                return n
            }
            return 0
        }

        // 对于 enip 协议，未知类型默认作为字符串处理
        return writeDialog.valueStr || ''
    }

    // ==================== 通用类型处理（其他协议） ====================

    // Boolean 类型
    if (isBoolType(dt)) {
        return writeDialog.valueBool
    }

    // String 类型
    if (isStringType(dt)) {
        return writeDialog.valueStr
    }

    // ByteString (二进制) 类型
    if (isBinaryType(dt)) {
        return {
            value: writeDialog.valueBinary,
            encoding: writeDialog.binaryEncoding
        }
    }

    // DateTime 类型
    if (isDateTimeType(dt)) {
        if (writeDialog.valueDateTime) {
            return writeDialog.valueDateTime.toISOString()
        }
        return new Date().toISOString()
    }

    // Guid 类型
    if (isGuidType(dt)) {
        return writeDialog.valueStr
    }

    // StatusCode 类型
    if (isStatusCodeType(dt)) {
        return writeDialog.valueStatusCode
    }

    // QualifiedName 类型
    if (isQualifiedNameType(dt)) {
        return writeDialog.valueStr
    }

    // LocalizedText 类型
    if (isLocalizedTextType(dt)) {
        return writeDialog.valueStr
    }

    // NodeId 类型
    if (isNodeIdType(dt)) {
        return writeDialog.valueStr
    }

    // Float/Double 类型 - 支持字符串形式的数值输入
    if (isFloatType(dt)) {
        const inputVal = writeDialog.valueStr !== undefined ? writeDialog.valueStr : writeDialog.valueNum
        const n = parseFloat(inputVal)
        if (!isNaN(n) && isFinite(n)) {
            return n
        }
        // 如果输入为空或无效，返回 0
        return 0
    }

    // Integer 类型 (Int8, Int16, Int32, Int64, UInt8, UInt16, UInt32, UInt64)
    if (isIntType(dt)) {
        const inputVal = writeDialog.valueStr !== undefined ? writeDialog.valueStr : writeDialog.valueNum
        const n = parseInt(inputVal)
        return isNaN(n) ? 0 : n
    }

    // 默认返回字符串或数值
    return writeDialog.valueStr || (writeDialog.valueNum !== undefined ? writeDialog.valueNum : 0)
}
</script>

<style scoped>
/* v3.0 — styles in src/styles/ */

.table-toolbar--points {
  padding: var(--space-2) 0;
}

.left-title {
  font-size: 12px;
  font-weight: 600;
  letter-spacing: 0.08em;
  text-transform: uppercase;
  color: var(--text-tertiary, #94a3b8);
  display: inline-flex;
  align-items: center;
  gap: var(--space-2);
}

.left-title::before {
  content: '';
  width: 3px;
  height: 12px;
  border-radius: 2px;
  background: var(--primary, rgb(22, 93, 255));
}
</style>