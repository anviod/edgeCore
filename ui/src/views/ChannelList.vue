<template>
  <div class="page-shell page-shell--wide channel-list-page channel-list-container">
    <div class="page-header channel-header">
      <div class="header-title">
        <h2 class="page-title title-text">采集通道</h2>
        <div class="page-subtitle title-subtitle">管理工业设备通信通道及协议配置</div>
      </div>
      <div class="header-actions channel-header-actions">
        <a-space size="small">
          <a-radio-group v-model="viewMode" type="button" size="small">
            <a-tooltip content="卡片视图">
              <a-radio value="card">
                <svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2">
                  <rect x="3" y="3" width="7" height="7" />
                  <rect x="14" y="3" width="7" height="7" />
                  <rect x="14" y="14" width="7" height="7" />
                  <rect x="3" y="14" width="7" height="7" />
                </svg>
              </a-radio>
            </a-tooltip>
            <a-tooltip content="列表视图">
              <a-radio value="list">
                <svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2">
                  <line x1="8" y1="6" x2="21" y2="6" />
                  <line x1="8" y1="12" x2="21" y2="12" />
                  <line x1="8" y1="18" x2="21" y2="18" />
                  <line x1="3" y1="6" x2="3.01" y2="6" />
                  <line x1="3" y1="12" x2="3.01" y2="12" />
                  <line x1="3" y1="18" x2="3.01" y2="18" />
                </svg>
              </a-radio>
            </a-tooltip>
          </a-radio-group>
        </a-space>
        <a-space size="small" wrap>
          <a-dropdown v-if="selectionMode && selectedChannels.length > 0" @select="onBatchConfigSelect" :trigger="['click']">
            <a-button status="warning" size="small" :loading="batchConfigLoading">
              <template #icon>
                <svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2">
                  <circle cx="12" cy="12" r="3" />
                  <path d="M19.4 15a1.65 1.65 0 00.33 1.82l.06.06a2 2 0 010 2.83 2 2 0 01-2.83 0l-.06-.06a1.65 1.65 0 00-1.82-.33 1.65 1.65 0 00-1 1.51V21a2 2 0 01-2 2 2 2 0 01-2-2v-.09A1.65 1.65 0 009 19.4a1.65 1.65 0 00-1.82.33l-.06.06a2 2 0 01-2.83 0 2 2 0 010-2.83l.06-.06a1.65 1.65 0 00.33-1.82 1.65 1.65 0 00-1.51-1H3a2 2 0 01-2-2 2 2 0 012-2h.09A1.65 1.65 0 004.6 9a1.65 1.65 0 00-.33-1.82l-.06-.06a2 2 0 010-2.83 2 2 0 012.83 0l.06.06a1.65 1.65 0 001.82.33H9a1.65 1.65 0 001-1.51V3a2 2 0 012-2 2 2 0 012 2v.09a1.65 1.65 0 001 1.51 1.65 1.65 0 001.82-.33l.06-.06a2 2 0 012.83 0 2 2 0 010 2.83l-.06.06a1.65 1.65 0 00-.33 1.82V9a1.65 1.65 0 001.51 1H21a2 2 0 012 2 2 2 0 01-2 2h-.09a1.65 1.65 0 00-1.51 1z" />
                </svg>
              </template>批量配置
              <template #icon-arrow-right><icon-down :size="12" /></template>
            </a-button>
            <template #content>
              <a-doption value="enable"><icon-check-circle :size="14" /> 批量启用</a-doption>
              <a-doption value="disable"><icon-minus-circle :size="14" /> 批量禁用</a-doption>
              <a-doption value="delete" status="danger"><icon-delete :size="14" /> 批量删除</a-doption>
            </template>
          </a-dropdown>
          <a-button :type="selectionMode ? 'secondary' : 'outline'" size="small" @click="toggleSelectionMode">
            <template #icon>
              <svg v-if="selectionMode" viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2">
                <line x1="18" y1="6" x2="6" y2="18" />
                <line x1="6" y1="6" x2="18" y2="18" />
              </svg>
              <svg v-else viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2">
                <rect x="3" y="3" width="18" height="18" rx="2" ry="2" />
                <polyline points="9 11 12 14 22 4" />
              </svg>
            </template>
            {{ selectionMode ? '取消选择' : '批量操作' }}
          </a-button>
          <a-button type="outline" size="small" :loading="loading" @click="fetchChannels">
            <template #icon>
              <svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2">
                <polyline points="23 4 23 10 17 10" />
                <polyline points="1 20 1 14 7 14" />
                <path d="M3.51 9a9 9 0 0114.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0020.49 15" />
              </svg>
            </template>刷新
          </a-button>
          <a-button type="primary" size="small" @click="openAddDialog">
            <template #icon>
              <svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2">
                <line x1="12" y1="5" x2="12" y2="19" />
                <line x1="5" y1="12" x2="19" y2="12" />
              </svg>
            </template>添加通道
          </a-button>
          <a-button type="text" size="small" class="help-trigger-btn" @click="openChannelHelp">
            <template #icon>
              <svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2">
                <circle cx="12" cy="12" r="10" />
                <path d="M9.09 9a3 3 0 015.83 1c0 2-3 3-3 3" />
                <line x1="12" y1="17" x2="12.01" y2="17" />
              </svg>
            </template>
            帮助说明
          </a-button>
        </a-space>
      </div>
    </div>

    <div class="channel-list-body">
      <a-spin :loading="loading" tip="数据同步中..." style="width: 100%">
        <div v-if="channels.length === 0 && !loading" class="empty-card">
          <div class="empty-content">
            <icon-apps :size="48" style="margin-bottom: 12px;" />
            <p>暂无采集通道配置</p>
            <a-button type="primary" @click="openAddDialog">添加通道</a-button>
          </div>
        </div>

        <template v-else>
          <section
            v-for="zone in channelZones"
            :key="zone.key"
            class="channel-list-zone"
            :aria-label="`${zone.title}通道`"
          >
            <div class="channel-list-zone-header">
              <h3 class="channel-list-zone-title">
                {{ zone.title }}
                <span class="channel-list-zone-count">{{ zone.items.length }}</span>
              </h3>
            </div>

            <div class="channel-list-tertiary-block">
              <div class="section">
                <div v-if="viewMode === 'card'" class="channels-grid">
                  <div
                    v-for="item in zone.items"
                    :key="item.id"
                    class="channel-card"
                    :class="[getProtocolClass(item.protocol), { 'is-selected': isSelected(item.id) }]"
                    @click="handleCardClick(item)"
                  >
                    <div class="channel-card-inner">
                      <div class="channel-card-top">
                        <div class="channel-identity">
                          <div class="channel-icon" :class="getProtocolClass(item.protocol)">
                            <icon-link v-if="['bacnet-ip', 'modbus-rtu', 'modbus-tcp', 'modbus-rtu-over-tcp'].includes(item.protocol)" :size="16" />
                            <icon-tool v-else-if="item.protocol === 'opc-ua'" :size="16" />
                            <icon-settings v-else-if="item.protocol === 's7'" :size="16" />
                            <icon-link v-else :size="16" />
                          </div>
                          <div class="channel-info">
                            <div class="channel-name">{{ item.name }}</div>
                            <div class="channel-meta">
                              <span class="protocol-tag">{{ formatProtocolTag(item.protocol) }}</span>
                              <span
                                class="channel-status-chip"
                                :class="item.enable ? 'is-enabled' : 'is-disabled'"
                              >
                                <span class="status-dot"></span>
                                {{ item.enableText }}
                              </span>
                            </div>
                          </div>
                        </div>
                        <span
                          v-if="item.metrics"
                          class="quality-score"
                          :class="getQualityClass(item.qualityScore)"
                          :title="`通信质量 ${item.qualityScore ?? '-'}`"
                        >
                          {{ item.qualityScore ?? '-' }}
                        </span>
                      </div>

                      <div class="channel-kpi-grid">
                        <div class="channel-kpi">
                          <span class="channel-kpi-label">设备</span>
                          <span class="channel-kpi-value">{{ item.deviceCount || 0 }}</span>
                        </div>
                        <div class="channel-kpi">
                          <span class="channel-kpi-label">状态</span>
                          <span
                            class="channel-kpi-value"
                            :class="item.runtimeArcoStatus === 'success' ? 'online' : (item.runtimeArcoStatus === 'danger' ? 'offline' : '')"
                          >
                            {{ formatCardRuntimeText(item.runtimeText) }}
                          </span>
                        </div>
                        <div class="channel-kpi">
                          <span class="channel-kpi-label">成功率</span>
                          <span class="channel-kpi-value" :class="getSuccessRateClass(item.successRate)">
                            {{ formatPercent(item.successRate) }}
                          </span>
                        </div>
                      </div>

                      <div v-if="item.metrics" class="channel-metrics">
                        <div class="metrics-header">
                          <span class="metrics-label">通信质量</span>
                          <span class="metrics-rtt">RTT {{ formatDuration(item.metrics.avgRtt) }}</span>
                        </div>
                        <div class="quality-bar-container">
                          <div class="quality-bar" :class="getQualityBarClass(item.qualityScore)" :style="{ width: (item.qualityScore || 0) + '%' }"></div>
                        </div>
                      </div>

                      <div class="channel-card-footer" @click.stop>
                        <div class="channel-card-actions">
                          <a-tooltip content="监控指标">
                            <a-button type="text" size="mini" @click.stop="openMetricsDialog(item)">
                              <svg viewBox="0 0 24 24" width="14" height="14" fill="none" stroke="currentColor" stroke-width="2">
                                <line x1="18" y1="20" x2="18" y2="10" />
                                <line x1="12" y1="20" x2="12" y2="4" />
                                <line x1="6" y1="20" x2="6" y2="14" />
                              </svg>
                            </a-button>
                          </a-tooltip>
                          <a-tooltip content="编辑">
                            <a-button type="text" size="mini" @click.stop="openEditDialog(item)">
                              <svg viewBox="0 0 24 24" width="14" height="14" fill="none" stroke="currentColor" stroke-width="2">
                                <path d="M11 4H4a2 2 0 00-2 2v14a2 2 0 002 2h14a2 2 0 002-2v-7" />
                                <path d="M18.5 2.5a2.121 2.121 0 013 3L12 15l-4 1 1-4 9.5-9.5z" />
                              </svg>
                            </a-button>
                          </a-tooltip>
                          <a-tooltip v-if="item.protocol === 'bacnet-ip'" content="扫描设备">
                            <a-button type="text" size="mini" @click.stop="scanChannel(item)">
                              <svg viewBox="0 0 24 24" width="14" height="14" fill="none" stroke="currentColor" stroke-width="2">
                                <path d="M12 2a10 10 0 1010 10 10 10 0 00-10-10z" />
                                <path d="M12 6v6l4 2" />
                              </svg>
                            </a-button>
                          </a-tooltip>
                          <a-tooltip content="删除">
                            <a-button type="text" size="mini" status="danger" @click.stop="deleteChannel(item)">
                              <svg viewBox="0 0 24 24" width="14" height="14" fill="none" stroke="currentColor" stroke-width="2">
                                <polyline points="3 6 5 6 21 6" />
                                <path d="M19 6v14a2 2 0 01-2 2H7a2 2 0 01-2-2V6m3 0V4a2 2 0 012-2h4a2 2 0 012 2v2" />
                              </svg>
                            </a-button>
                          </a-tooltip>
                        </div>
                      </div>
                    </div>
                  </div>

                  <div v-if="zone.items.length === 0" class="channel-list-zone-empty">
                    暂无{{ zone.title }}通道
                  </div>
                </div>

                <div v-else-if="zone.items.length > 0" class="table-container saas-table">
                  <a-table
                    :columns="tableColumns"
                    :data="zone.items"
                    :row-selection="selectionMode ? rowSelection : undefined"
                    row-key="id"
                    size="small"
                    :bordered="false"
                    :scroll="{ x: 960 }"
                    :pagination="false"
                  >
                    <template #name="{ record }">
                      <a-link @click="goToDevices(record)" icon>{{ record.name }}</a-link>
                    </template>

                    <template #enable="{ record }">
                      <span class="table-cell-semantic">
                        <a-tag :color="record.enableColor" size="small" bordered>{{ record.enableText }}</a-tag>
                      </span>
                    </template>

                    <template #runtime="{ record }">
                      <span class="table-cell-semantic">
                        <a-badge :status="record.runtimeArcoStatus" :text="record.runtimeText" />
                      </span>
                    </template>

                    <template #deviceCount="{ record }">
                      <a-tooltip :content="`${record.deviceCount} 台设备`">
                        <span class="table-cell-count">{{ record.deviceCount }}</span>
                      </a-tooltip>
                    </template>

                    <template #actions="{ record }">
                      <span class="table-ops">
                        <a-tooltip content="监控">
                          <a-button type="text" size="mini" @click="openMetricsDialog(record)">
                            <svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2">
                              <line x1="18" y1="20" x2="18" y2="10" />
                              <line x1="12" y1="20" x2="12" y2="4" />
                              <line x1="6" y1="20" x2="6" y2="14" />
                            </svg>
                          </a-button>
                        </a-tooltip>
                        <a-tooltip content="编辑">
                          <a-button type="text" size="mini" @click="openEditDialog(record)">
                            <svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2">
                              <path d="M11 4H4a2 2 0 00-2 2v14a2 2 0 002 2h14a2 2 0 002-2v-7" />
                              <path d="M18.5 2.5a2.121 2.121 0 013 3L12 15l-4 1 1-4 9.5-9.5z" />
                            </svg>
                          </a-button>
                        </a-tooltip>
                        <a-tooltip v-if="record.protocol === 'bacnet-ip'" content="扫描">
                          <a-button type="text" size="mini" @click="scanChannel(record)">
                            <svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2">
                              <path d="M12 2a10 10 0 1010 10 10 10 0 00-10-10z" />
                              <path d="M12 6v6l4 2" />
                            </svg>
                          </a-button>
                        </a-tooltip>
                        <a-tooltip content="删除">
                          <a-button type="text" size="mini" status="danger" @click="deleteChannel(record)">
                            <svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2">
                              <polyline points="3 6 5 6 21 6" />
                              <path d="M19 6v14a2 2 0 01-2 2H7a2 2 0 01-2-2V6m3 0V4a2 2 0 012-2h4a2 2 0 012 2v2" />
                            </svg>
                          </a-button>
                        </a-tooltip>
                      </span>
                    </template>
                  </a-table>
                </div>

                <div v-else class="channel-list-zone-empty">
                  暂无{{ zone.title }}通道
                </div>
              </div>
            </div>
          </section>
        </template>
      </a-spin>
    </div>

    <!-- Add/Edit Dialog -->
    <a-modal 
      v-model:visible="dialog.show" 
      :title="dialog.isEdit ? '编辑通道' : '添加通道'"
      :width="760"
      modal-class="channel-config-modal"
      @before-ok="saveChannel"
    >
      <a-form :model="dialog.form" layout="vertical" class="channel-config-form flow-form form-controls-md">
        <a-form-item field="id" label="ID" required>
          <a-input v-model="dialog.form.id" :disabled="dialog.isEdit" placeholder="请输入ID">
            <template #append v-if="!dialog.isEdit">
              <a-button @click="generateId">
                <svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2">
                  <polyline points="23 4 23 10 17 10" />
                  <polyline points="1 20 1 14 7 14" />
                  <path d="M3.51 9a9 9 0 0114.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0020.49 15" />
                </svg>
              </a-button>
            </template>
          </a-input>
        </a-form-item>
        
        <a-form-item field="name" label="名称" required>
          <a-input v-model="dialog.form.name" placeholder="给通道起一个易于识别的名称" />
        </a-form-item>

        <a-form-item field="protocol" label="协议" required>
          <a-select v-model="dialog.form.protocol" :options="protocols" placeholder="请选择协议" />
        </a-form-item>

        <a-form-item field="enable" label="启用">
          <a-switch v-model="dialog.form.enable" />
        </a-form-item>

        <div class="modal-section__title config-section-header">
          <span>协议配置</span>
          <a-tooltip content="用推荐值填充尚未填写的配置项，不会覆盖已有内容">
            <a-button type="outline" size="mini" :disabled="!dialog.form.protocol" @click="fillDefaultConfig">
              <template #icon>
                <svg viewBox="0 0 24 24" width="14" height="14" fill="none" stroke="currentColor" stroke-width="2">
                  <path d="M12 2v4M12 18v4M4.93 4.93l2.83 2.83M16.24 16.24l2.83 2.83M2 12h4M18 12h4M4.93 19.07l2.83-2.83M16.24 7.76l2.83-2.83" />
                </svg>
              </template>
              填充默认参数
            </a-button>
          </a-tooltip>
        </div>

        <!-- Modbus TCP & Modbus RTU Over TCP Config -->
        <div v-if="dialog.form.protocol === 'modbus-tcp' || dialog.form.protocol === 'modbus-rtu-over-tcp'" class="config-section">
          <a-form-item field="config.url" :label="dialog.form.protocol === 'modbus-rtu-over-tcp' ? 'URL (tcp+rtu://ip:port)' : 'URL (tcp://ip:port)'">
            <a-input v-model="dialog.form.config.url" />
          </a-form-item>
          <a-form-item field="config.timeout" label="超时时间 (ms)">
            <a-input-number v-model="dialog.form.config.timeout" :min="100" :max="60000" placeholder="2000" />
          </a-form-item>

          <div class="advanced-block">
            <div class="modal-section__title modal-section__title--sub">高级配置</div>
            <a-row :gutter="[24, 16]" class="field-grid">
              <a-col :span="12">
                <a-form-item field="config.max_retries" label="最大重试次数">
                  <a-input-number v-model="dialog.form.config.max_retries" :min="0" :max="10" placeholder="3" />
                </a-form-item>
              </a-col>
              <a-col :span="12">
                <a-form-item field="config.retry_interval" label="重试间隔 (ms)">
                  <a-input-number v-model="dialog.form.config.retry_interval" :min="10" :max="1000" placeholder="100" />
                </a-form-item>
              </a-col>
              <a-col :span="12">
                <a-form-item field="config.instruction_interval" label="指令间隔 (ms)">
                  <a-input-number v-model="dialog.form.config.instruction_interval" :min="1" :max="100" placeholder="10" />
                </a-form-item>
              </a-col>
              <a-col :span="12">
                <a-form-item field="config.start_address" label="起始地址">
                  <a-select v-model="dialog.form.config.start_address" placeholder="默认 1 (40001)">
                    <a-option :value="0">0 (40000)</a-option>
                    <a-option :value="1">1 (40001)</a-option>
                  </a-select>
                </a-form-item>
              </a-col>
              <a-col :span="12">
                <a-form-item field="config.byte_order_4" label="4字节字节序">
                  <a-select v-model="dialog.form.config.byte_order_4" placeholder="默认 ABCD (Big Endian)">
                    <a-option value="ABCD">ABCD</a-option>
                    <a-option value="CDAB">CDAB</a-option>
                    <a-option value="BADC">BADC</a-option>
                    <a-option value="DCBA">DCBA</a-option>
                  </a-select>
                </a-form-item>
              </a-col>
            </a-row>
          </div>
        </div>

        <!-- DLT645 Config -->
        <div v-if="dialog.form.protocol === 'dlt645'" class="config-section">
          <a-form-item field="config.connectionType" label="连接方式">
            <a-select v-model="dialog.form.config.connectionType" placeholder="请选择连接方式">
              <a-option value="serial">串口 (Serial)</a-option>
              <a-option value="tcp">网络 (TCP)</a-option>
            </a-select>
          </a-form-item>
          <div v-if="dialog.form.config.connectionType === 'tcp'">
            <a-form-item field="config.ip" label="设备 IP 地址">
              <a-input v-model="dialog.form.config.ip" placeholder="192.168.1.100" />
            </a-form-item>
            <a-form-item field="config.port" label="端口">
              <a-input-number v-model="dialog.form.config.port" :min="1" :max="65535" placeholder="8001" />
            </a-form-item>
            <a-form-item field="config.timeout" label="超时时间 (ms)">
              <a-input-number v-model="dialog.form.config.timeout" :min="100" :max="60000" placeholder="2000" />
            </a-form-item>
          </div>
        </div>

        <!-- Modbus RTU & DLT645 Serial Config -->
        <div v-if="dialog.form.protocol === 'modbus-rtu' || (dialog.form.protocol === 'dlt645' && dialog.form.config.connectionType === 'serial')" class="config-section">
          <a-form-item field="config.port" label="串口设备">
            <a-input v-model="dialog.form.config.port" placeholder="/dev/ttyS1" />
          </a-form-item>
          <a-row :gutter="[24, 16]" class="field-grid">
            <a-col :span="6">
              <a-form-item field="config.baudRate" label="波特率">
                <a-select v-model="dialog.form.config.baudRate" placeholder="9600">
                  <a-option :value="1200">1200</a-option>
                  <a-option :value="2400">2400</a-option>
                  <a-option :value="4800">4800</a-option>
                  <a-option :value="9600">9600</a-option>
                  <a-option :value="19200">19200</a-option>
                  <a-option :value="38400">38400</a-option>
                  <a-option :value="57600">57600</a-option>
                  <a-option :value="115200">115200</a-option>
                </a-select>
              </a-form-item>
            </a-col>
            <a-col :span="6">
              <a-form-item field="config.dataBits" label="数据位">
                <a-select v-model="dialog.form.config.dataBits" placeholder="8">
                  <a-option :value="5">5</a-option>
                  <a-option :value="6">6</a-option>
                  <a-option :value="7">7</a-option>
                  <a-option :value="8">8</a-option>
                </a-select>
              </a-form-item>
            </a-col>
            <a-col :span="6">
              <a-form-item field="config.stopBits" label="停止位">
                <a-select v-model="dialog.form.config.stopBits" placeholder="1">
                  <a-option :value="1">1</a-option>
                  <a-option :value="2">2</a-option>
                </a-select>
              </a-form-item>
            </a-col>
            <a-col :span="6">
              <a-form-item field="config.parity" label="校验位">
                <a-select v-model="dialog.form.config.parity" placeholder="无校验">
                  <a-option value="N">无校验 (None)</a-option>
                  <a-option value="E">偶校验 (Even)</a-option>
                  <a-option value="O">奇校验 (Odd)</a-option>
                </a-select>
              </a-form-item>
            </a-col>
          </a-row>
          <a-form-item field="config.timeout" label="超时时间 (ms)">
            <a-input-number v-model="dialog.form.config.timeout" :min="100" :max="60000" placeholder="2000" />
          </a-form-item>

          <!-- Modbus RTU Advanced Config -->
          <div v-if="dialog.form.protocol === 'modbus-rtu'" class="advanced-block">
            <div class="modal-section__title modal-section__title--sub">高级配置</div>
            <a-row :gutter="[24, 16]" class="field-grid">
              <a-col :span="12">
                <a-form-item field="config.max_retries" label="最大重试次数">
                  <a-input-number v-model="dialog.form.config.max_retries" :min="0" :max="10" placeholder="3" />
                </a-form-item>
              </a-col>
              <a-col :span="12">
                <a-form-item field="config.retry_interval" label="重试间隔 (ms)">
                  <a-input-number v-model="dialog.form.config.retry_interval" :min="10" :max="1000" placeholder="100" />
                </a-form-item>
              </a-col>
              <a-col :span="12">
                <a-form-item field="config.instruction_interval" label="指令间隔 (ms)">
                  <a-input-number v-model="dialog.form.config.instruction_interval" :min="1" :max="100" placeholder="10" />
                </a-form-item>
              </a-col>
              <a-col :span="12">
                <a-form-item field="config.start_address" label="起始地址">
                  <a-select v-model="dialog.form.config.start_address" placeholder="默认 1 (40001)">
                    <a-option :value="0">0 (40000)</a-option>
                    <a-option :value="1">1 (40001)</a-option>
                  </a-select>
                </a-form-item>
              </a-col>
              <a-col :span="12">
                <a-form-item field="config.byte_order_4" label="4字节字节序">
                  <a-select v-model="dialog.form.config.byte_order_4" placeholder="默认 ABCD (Big Endian)">
                    <a-option value="ABCD">ABCD</a-option>
                    <a-option value="CDAB">CDAB</a-option>
                    <a-option value="BADC">BADC</a-option>
                    <a-option value="DCBA">DCBA</a-option>
                  </a-select>
                </a-form-item>
              </a-col>
            </a-row>
          </div>
        </div>

        <!-- BACnet IP Config -->
        <div v-if="dialog.form.protocol === 'bacnet-ip'" class="config-section">
          <a-row :gutter="[24, 16]" class="field-grid">
            <a-col :span="12">
              <a-form-item field="config.interface_ip" label="本机IP (绑定)">
                <a-input v-model="dialog.form.config.interface_ip" placeholder="自动获取" />
              </a-form-item>
            </a-col>
            <a-col :span="12">
              <a-form-item field="config.target_ip" label="远程目标IP">
                <a-input v-model="dialog.form.config.target_ip" placeholder="例: 192.168.3.115" />
              </a-form-item>
            </a-col>
          </a-row>
          <a-row :gutter="[24, 16]" class="field-grid">
            <a-col :span="12">
              <a-form-item field="config.port" label="端口">
                <a-input-number v-model="dialog.form.config.port" :min="1" :max="65535" placeholder="47808 (默认)" />
              </a-form-item>
            </a-col>
          </a-row>
          <div class="advanced-block">
            <div class="modal-section__title modal-section__title--sub">加密参数 (可选)</div>
            <a-form-item field="config.key" label="密钥">
              <a-input v-model="dialog.form.config.key" type="password" />
            </a-form-item>
            <a-form-item field="config.cert" label="证书路径">
              <a-input v-model="dialog.form.config.cert" />
            </a-form-item>
            <a-form-item field="config.ca" label="CA证书路径">
              <a-input v-model="dialog.form.config.ca" />
            </a-form-item>
          </div>
        </div>

        <!-- OPC UA Config -->
        <div v-if="dialog.form.protocol === 'opc-ua'" class="config-section">
          <a-form-item field="config.url" label="Endpoint URL">
            <a-input v-model="dialog.form.config.url" placeholder="opc.tcp://localhost:4840" />
          </a-form-item>
        </div>

        <!-- S7 Config -->
        <div v-if="dialog.form.protocol === 's7'" class="config-section">
          <div class="modal-section__title modal-section__title--sub">基础连接</div>
          <a-form-item field="config.ip" label="PLC IP 地址" required>
            <a-input v-model="dialog.form.config.ip" placeholder="192.168.1.10" />
          </a-form-item>
          <a-form-item field="config.port" label="PLC 端口">
            <a-input-number v-model="dialog.form.config.port" :min="1" :max="65535" placeholder="102 (默认)" />
          </a-form-item>
          <a-row :gutter="[24, 16]" class="field-grid">
            <a-col :span="8">
              <a-form-item field="config.plcType" label="PLC 型号">
                <a-select v-model="dialog.form.config.plcType" placeholder="请选择" allow-clear>
                  <a-option value="S7-200Smart">S7-200Smart</a-option>
                  <a-option value="S7-1200">S7-1200</a-option>
                  <a-option value="S7-1500">S7-1500</a-option>
                  <a-option value="S7-300">S7-300</a-option>
                  <a-option value="S7-400">S7-400</a-option>
                </a-select>
              </a-form-item>
            </a-col>
            <a-col :span="8">
              <a-form-item field="config.rack" label="机架号 (Rack)">
                <a-input-number v-model="dialog.form.config.rack" :min="0" :max="10" placeholder="自动" />
              </a-form-item>
            </a-col>
            <a-col :span="8">
              <a-form-item field="config.slot" label="槽号 (Slot)">
                <a-input-number v-model="dialog.form.config.slot" :min="0" :max="10" placeholder="自动" />
              </a-form-item>
            </a-col>
          </a-row>
          <a-row :gutter="[24, 16]" class="field-grid">
            <a-col :span="8">
              <a-form-item field="config.connect_type" label="连接类型">
                <a-select v-model="dialog.form.config.connect_type" placeholder="自动" allow-clear>
                  <a-option value="PG">PG (编程设备)</a-option>
                  <a-option value="OP">OP (操作面板)</a-option>
                  <a-option value="S7Basic">S7 Basic</a-option>
                </a-select>
              </a-form-item>
            </a-col>
            <a-col :span="8">
              <a-form-item field="config.startup" label="启动方式">
                <a-select v-model="dialog.form.config.startup" placeholder="请选择" allow-clear>
                  <a-option value="cold">冷启动</a-option>
                  <a-option value="warm">热启动</a-option>
                </a-select>
              </a-form-item>
            </a-col>
            <a-col :span="8">
              <a-form-item field="config.cpu_protection" label="CPU停机保护">
                <a-switch v-model="dialog.form.config.cpu_protection" />
              </a-form-item>
            </a-col>
          </a-row>

          <div class="modal-section__title modal-section__title--sub">通信参数</div>
          <a-row :gutter="[24, 16]" class="field-grid">
            <a-col :span="8">
              <a-form-item field="config.timeout" label="超时时间 (ms)">
                <a-input-number v-model="dialog.form.config.timeout" :min="500" :max="30000" placeholder="2000" />
              </a-form-item>
            </a-col>
            <a-col :span="8">
              <a-form-item field="config.max_retries" label="重试次数">
                <a-input-number v-model="dialog.form.config.max_retries" :min="0" :max="10" placeholder="1" />
              </a-form-item>
            </a-col>
            <a-col :span="8">
              <a-form-item field="config.heartbeat_interval" label="心跳间隔 (ms)">
                <a-input-number v-model="dialog.form.config.heartbeat_interval" :min="0" :max="300000" placeholder="30000" />
              </a-form-item>
            </a-col>
          </a-row>
          <a-row :gutter="[24, 16]" class="field-grid">
            <a-col :span="8">
              <a-form-item field="config.pdu_size" label="PDU缓冲区 (字节)">
                <a-input-number v-model="dialog.form.config.pdu_size" :min="240" :max="960" placeholder="4096" />
              </a-form-item>
            </a-col>
            <a-col :span="8">
              <a-form-item field="config.qos" label="QoS 等级">
                <a-select v-model="dialog.form.config.qos" placeholder="1" allow-clear>
                  <a-option :value="0">0 - 最多一次</a-option>
                  <a-option :value="1">1 - 至少一次</a-option>
                  <a-option :value="2">2 - 恰好一次</a-option>
                </a-select>
              </a-form-item>
            </a-col>
            <a-col :span="8">
              <a-form-item field="config.connect_timeout" label="连接超时 (ms)">
                <a-input-number v-model="dialog.form.config.connect_timeout" :min="500" :max="60000" placeholder="5000" />
              </a-form-item>
            </a-col>
          </a-row>
          <a-row :gutter="[24, 16]" class="field-grid">
            <a-col :span="8">
              <a-form-item field="config.batch_read_max" label="批量读取上限">
                <a-input-number v-model="dialog.form.config.batch_read_max" :min="1" :max="500" placeholder="100" />
              </a-form-item>
            </a-col>
          </a-row>
        </div>

        <!-- Omron FINS Config -->
        <div v-if="dialog.form.protocol === 'omron-fins'" class="config-section">
          <div class="modal-section__title modal-section__title--sub">基础连接</div>
          <a-form-item field="config.ip" label="PLC IP 地址" required>
            <a-input v-model="dialog.form.config.ip" placeholder="192.168.1.100" />
          </a-form-item>
          <a-row :gutter="[24, 16]" class="field-grid">
            <a-col :span="8">
              <a-form-item field="config.port" label="PLC 端口">
                <a-input-number v-model="dialog.form.config.port" :min="1" :max="65535" placeholder="9600 (默认)" />
              </a-form-item>
            </a-col>
            <a-col :span="8">
              <a-form-item field="config.mode" label="传输模式">
                <a-select v-model="dialog.form.config.mode" placeholder="TCP">
                  <a-option value="TCP">TCP</a-option>
                  <a-option value="UDP">UDP</a-option>
                </a-select>
              </a-form-item>
            </a-col>
            <a-col :span="8">
              <a-form-item field="config.model" label="PLC 型号">
                <a-select v-model="dialog.form.config.model" placeholder="请选择" allow-clear>
                  <a-option value="CP1E">CP1E</a-option>
                  <a-option value="CP1H">CP1H</a-option>
                  <a-option value="CJ">CJ 系列</a-option>
                  <a-option value="CS">CS 系列</a-option>
                  <a-option value="NJ">NJ 系列</a-option>
                </a-select>
              </a-form-item>
            </a-col>
          </a-row>

          <div class="modal-section__title modal-section__title--sub">FINS 节点地址</div>
          <a-row :gutter="[24, 16]" class="field-grid">
            <a-col :span="8">
              <a-form-item field="config.src_network_addr" label="源网络地址">
                <a-input-number v-model="dialog.form.config.src_network_addr" :min="0" :max="255" placeholder="0" />
              </a-form-item>
            </a-col>
            <a-col :span="8">
              <a-form-item field="config.src_node_addr" label="源节点地址">
                <a-input-number v-model="dialog.form.config.src_node_addr" :min="0" :max="255" placeholder="1" />
              </a-form-item>
            </a-col>
            <a-col :span="8">
              <a-form-item field="config.src_unit_addr" label="源单元地址">
                <a-input-number v-model="dialog.form.config.src_unit_addr" :min="0" :max="255" placeholder="255" />
              </a-form-item>
            </a-col>
          </a-row>
          <a-row :gutter="[24, 16]" class="field-grid">
            <a-col :span="8">
              <a-form-item field="config.dst_network_addr" label="目标网络地址">
                <a-input-number v-model="dialog.form.config.dst_network_addr" :min="0" :max="255" placeholder="0" />
              </a-form-item>
            </a-col>
            <a-col :span="8">
              <a-form-item field="config.dst_node_addr" label="目标节点地址">
                <a-input-number v-model="dialog.form.config.dst_node_addr" :min="0" :max="255" placeholder="PLC IP 末段" />
              </a-form-item>
            </a-col>
            <a-col :span="8">
              <a-form-item field="config.dst_unit_addr" label="目标单元地址">
                <a-input-number v-model="dialog.form.config.dst_unit_addr" :min="0" :max="255" placeholder="0" />
              </a-form-item>
            </a-col>
          </a-row>

          <div class="modal-section__title modal-section__title--sub">通信参数</div>
          <a-row :gutter="[24, 16]" class="field-grid">
            <a-col :span="8">
              <a-form-item field="config.timeout" label="超时时间 (ms)">
                <a-input-number v-model="dialog.form.config.timeout" :min="500" :max="30000" placeholder="3000" />
              </a-form-item>
            </a-col>
            <a-col :span="8">
              <a-form-item field="config.max_retries" label="重试次数">
                <a-input-number v-model="dialog.form.config.max_retries" :min="0" :max="10" placeholder="3" />
              </a-form-item>
            </a-col>
            <a-col :span="8">
              <a-form-item field="config.heartbeat_interval" label="心跳间隔 (ms)">
                <a-input-number v-model="dialog.form.config.heartbeat_interval" :min="0" :max="300000" placeholder="30000" />
              </a-form-item>
            </a-col>
          </a-row>
          <a-row :gutter="[24, 16]" class="field-grid">
            <a-col :span="8">
              <a-form-item field="config.maxFrameLength" label="批量读取字数上限">
                <a-input-number v-model="dialog.form.config.maxFrameLength" :min="1" :max="500" placeholder="64" />
              </a-form-item>
            </a-col>
            <a-col :span="8">
              <a-form-item field="config.min_interval" label="指令间隔 (ms)">
                <a-input-number v-model="dialog.form.config.min_interval" :min="0" :max="1000" placeholder="0" />
              </a-form-item>
            </a-col>
            <a-col :span="8" v-if="dialog.form.config.mode === 'UDP'">
              <a-form-item field="config.local_port" label="本地 UDP 端口">
                <a-input-number v-model="dialog.form.config.local_port" :min="0" :max="65535" placeholder="0 (自动)" />
              </a-form-item>
            </a-col>
          </a-row>
        </div>

        <!-- KNXnet/IP Config -->
        <div v-if="dialog.form.protocol === 'knxnet-ip'" class="config-section">
          <div class="modal-section__title modal-section__title--sub">基础连接</div>
          <a-form-item field="config.ip" label="网关 IP">
            <a-input v-model="dialog.form.config.ip" placeholder="192.168.1.50（启用自动发现时可留空）" />
          </a-form-item>
          <a-row :gutter="[24, 16]" class="field-grid">
            <a-col :span="8">
              <a-form-item field="config.port" label="端口">
                <a-input-number v-model="dialog.form.config.port" :min="1" :max="65535" placeholder="3671 (默认)" />
              </a-form-item>
            </a-col>
            <a-col :span="8">
              <a-form-item field="config.mode" label="传输模式">
                <a-select v-model="dialog.form.config.mode" placeholder="UDP">
                  <a-option value="UDP">UDP</a-option>
                  <a-option value="TCP">TCP</a-option>
                </a-select>
              </a-form-item>
            </a-col>
            <a-col :span="8">
              <a-form-item field="config.timeout" label="超时时间 (ms)">
                <a-input-number v-model="dialog.form.config.timeout" :min="100" :max="60000" placeholder="3000" />
              </a-form-item>
            </a-col>
          </a-row>

          <div class="modal-section__title modal-section__title--sub">通信参数</div>
          <a-row :gutter="[24, 16]" class="field-grid">
            <a-col :span="8">
              <a-form-item field="config.max_retries" label="重试次数">
                <a-input-number v-model="dialog.form.config.max_retries" :min="0" :max="10" placeholder="3" />
              </a-form-item>
            </a-col>
            <a-col :span="8">
              <a-form-item field="config.heartbeat_interval" label="心跳间隔 (ms)">
                <a-input-number v-model="dialog.form.config.heartbeat_interval" :min="0" :max="300000" placeholder="60000" />
              </a-form-item>
            </a-col>
            <a-col :span="8">
              <a-form-item field="config.local_ip" label="本地 IP (可选)">
                <a-input v-model="dialog.form.config.local_ip" placeholder="192.168.1.10" />
              </a-form-item>
            </a-col>
          </a-row>

          <div class="modal-section__title modal-section__title--sub">网关发现 (SEARCH)</div>
          <a-form-item field="config.discovery" label="启用自动发现">
            <a-switch v-model="dialog.form.config.discovery" />
          </a-form-item>
          <a-row v-if="dialog.form.config.discovery" :gutter="[24, 16]" class="field-grid">
            <a-col :span="12">
              <a-form-item field="config.discovery_timeout" label="发现超时 (ms)">
                <a-input-number v-model="dialog.form.config.discovery_timeout" :min="500" :max="30000" placeholder="5000" />
              </a-form-item>
            </a-col>
            <a-col :span="12">
              <a-form-item field="config.discovery_multicast" label="多播地址">
                <a-input v-model="dialog.form.config.discovery_multicast" placeholder="224.0.23.12:3671" />
              </a-form-item>
            </a-col>
          </a-row>
        </div>

        <!-- Profinet IO Config -->
        <div v-if="dialog.form.protocol === 'profinet-io'" class="config-section">
          <div class="modal-section__title modal-section__title--sub">基础连接</div>
          <a-form-item field="config.local_interface" label="本地网口" required>
            <a-input v-model="dialog.form.config.local_interface" placeholder="eth0" />
            <template #extra>绑定用于 PROFINET 通信的物理网卡，需裸机部署</template>
          </a-form-item>
          <a-row :gutter="[24, 16]" class="field-grid">
            <a-col :span="8">
              <a-form-item field="config.timeout" label="超时时间 (ms)">
                <a-input-number v-model="dialog.form.config.timeout" :min="100" :max="60000" placeholder="3000" />
              </a-form-item>
            </a-col>
            <a-col :span="8">
              <a-form-item field="config.max_retries" label="重试次数">
                <a-input-number v-model="dialog.form.config.max_retries" :min="0" :max="10" placeholder="3" />
              </a-form-item>
            </a-col>
            <a-col :span="8">
              <a-form-item field="config.heartbeat_interval" label="心跳间隔 (ms)">
                <a-input-number v-model="dialog.form.config.heartbeat_interval" :min="0" :max="300000" placeholder="30000" />
              </a-form-item>
            </a-col>
          </a-row>
          <a-form-item field="config.simulation" label="模拟模式">
            <a-switch v-model="dialog.form.config.simulation" />
            <template #extra>开发测试时使用，无需真实 PROFINET IO 设备</template>
          </a-form-item>
        </div>

        <!-- Mitsubishi MC Config -->
        <div v-if="dialog.form.protocol === 'mitsubishi-slmp'" class="config-section">
          <div class="modal-section__title modal-section__title--sub">基础连接</div>
          <a-form-item field="config.ip" label="PLC IP 地址" required>
            <a-input v-model="dialog.form.config.ip" placeholder="192.168.1.10" />
          </a-form-item>
          <a-row :gutter="[24, 16]" class="field-grid">
            <a-col :span="8">
              <a-form-item field="config.port" label="PLC 端口">
                <a-input-number v-model="dialog.form.config.port" :min="1" :max="65535" placeholder="5000 (默认)" />
              </a-form-item>
            </a-col>
            <a-col :span="8">
              <a-form-item field="config.frame_type" label="帧类型">
                <a-select v-model="dialog.form.config.frame_type" placeholder="3E">
                  <a-option value="3E">3E (Q/L/iQ-R)</a-option>
                </a-select>
              </a-form-item>
            </a-col>
            <a-col :span="8">
              <a-form-item field="config.station_no" label="站号">
                <a-input-number v-model="dialog.form.config.station_no" :min="0" :max="255" placeholder="0" />
              </a-form-item>
            </a-col>
          </a-row>
          <a-row :gutter="[24, 16]" class="field-grid">
            <a-col :span="8">
              <a-form-item field="config.network_no" label="网络号">
                <a-input-number v-model="dialog.form.config.network_no" :min="0" :max="255" placeholder="0" />
              </a-form-item>
            </a-col>
            <a-col :span="8">
              <a-form-item field="config.pc_no" label="PC 编号">
                <a-input-number v-model="dialog.form.config.pc_no" :min="0" :max="255" placeholder="255" />
              </a-form-item>
            </a-col>
          </a-row>

          <div class="modal-section__title modal-section__title--sub">通信参数</div>
          <a-row :gutter="[24, 16]" class="field-grid">
            <a-col :span="8">
              <a-form-item field="config.timeout" label="超时时间 (ms)">
                <a-input-number v-model="dialog.form.config.timeout" :min="500" :max="30000" placeholder="3000" />
              </a-form-item>
            </a-col>
            <a-col :span="8">
              <a-form-item field="config.max_retries" label="重试次数">
                <a-input-number v-model="dialog.form.config.max_retries" :min="0" :max="10" placeholder="2" />
              </a-form-item>
            </a-col>
            <a-col :span="8">
              <a-form-item field="config.batch_read_max" label="批量读取上限">
                <a-input-number v-model="dialog.form.config.batch_read_max" :min="1" :max="500" placeholder="64" />
              </a-form-item>
            </a-col>
          </a-row>
        </div>

        <!-- IEC 60870-5-104 Config -->
        <div v-if="dialog.form.protocol === 'iec60870-5-104'" class="config-section">
          <div class="modal-section__title modal-section__title--sub">基础连接</div>
          <a-form-item field="config.ip" label="设备 IP 地址" required>
            <a-input v-model="dialog.form.config.ip" placeholder="192.168.1.100" />
          </a-form-item>
          <a-row :gutter="[24, 16]" class="field-grid">
            <a-col :span="8">
              <a-form-item field="config.port" label="端口">
                <a-input-number v-model="dialog.form.config.port" :min="1" :max="65535" placeholder="2404 (默认)" />
              </a-form-item>
            </a-col>
            <a-col :span="8">
              <a-form-item field="config.commonAddress" label="公共地址 CA">
                <a-input-number v-model="dialog.form.config.commonAddress" :min="1" :max="65535" placeholder="1" />
              </a-form-item>
            </a-col>
            <a-col :span="8">
              <a-form-item field="config.generalCallInterval" label="总召唤间隔 (秒)">
                <a-input-number v-model="dialog.form.config.generalCallInterval" :min="0" :max="86400" placeholder="300" />
              </a-form-item>
            </a-col>
          </a-row>
          <div class="modal-section__title modal-section__title--sub">协议定时器</div>
          <a-row :gutter="[24, 16]" class="field-grid">
            <a-col :span="6">
              <a-form-item field="config.t0" label="T0 (秒)">
                <a-input-number v-model="dialog.form.config.t0" :min="1" :max="120" placeholder="10" />
              </a-form-item>
            </a-col>
            <a-col :span="6">
              <a-form-item field="config.t1" label="T1 (秒)">
                <a-input-number v-model="dialog.form.config.t1" :min="1" :max="120" placeholder="15" />
              </a-form-item>
            </a-col>
            <a-col :span="6">
              <a-form-item field="config.t2" label="T2 (秒)">
                <a-input-number v-model="dialog.form.config.t2" :min="1" :max="120" placeholder="10" />
              </a-form-item>
            </a-col>
            <a-col :span="6">
              <a-form-item field="config.t3" label="T3 (秒)">
                <a-input-number v-model="dialog.form.config.t3" :min="1" :max="120" placeholder="20" />
              </a-form-item>
            </a-col>
          </a-row>
        </div>

        <!-- SNMP Config -->
        <div v-if="dialog.form.protocol === 'snmp'" class="config-section">
          <div class="modal-section__title modal-section__title--sub">基础连接</div>
          <a-form-item field="config.ip" label="设备 IP 地址" required>
            <a-input v-model="dialog.form.config.ip" placeholder="192.168.1.1" />
          </a-form-item>
          <a-row :gutter="[24, 16]" class="field-grid">
            <a-col :span="8">
              <a-form-item field="config.port" label="端口">
                <a-input-number v-model="dialog.form.config.port" :min="1" :max="65535" placeholder="161 (默认)" />
              </a-form-item>
            </a-col>
            <a-col :span="8">
              <a-form-item field="config.snmpVersion" label="SNMP 版本">
                <a-select v-model="dialog.form.config.snmpVersion" placeholder="v2c">
                  <a-option value="v2c">v2c</a-option>
                  <a-option value="v3">v3</a-option>
                </a-select>
              </a-form-item>
            </a-col>
            <a-col :span="8">
              <a-form-item field="config.timeout" label="超时 (ms)">
                <a-input-number v-model="dialog.form.config.timeout" :min="500" :max="60000" placeholder="3000" />
              </a-form-item>
            </a-col>
          </a-row>
          <a-row :gutter="[24, 16]" class="field-grid">
            <a-col :span="8">
              <a-form-item field="config.retries" label="重试次数">
                <a-input-number v-model="dialog.form.config.retries" :min="0" :max="10" placeholder="3" />
              </a-form-item>
            </a-col>
            <a-col :span="8">
              <a-form-item field="config.maxBulkSize" label="GETBULK 数量">
                <a-input-number v-model="dialog.form.config.maxBulkSize" :min="1" :max="100" placeholder="10" />
              </a-form-item>
            </a-col>
            <a-col :span="8">
              <a-form-item field="config.sendInterval" label="发送间隔 (ms)">
                <a-input-number v-model="dialog.form.config.sendInterval" :min="0" :max="5000" placeholder="100" />
              </a-form-item>
            </a-col>
          </a-row>

          <div v-if="!dialog.form.config.snmpVersion || dialog.form.config.snmpVersion === 'v2c'">
            <div class="modal-section__title modal-section__title--sub">SNMP v2c</div>
            <a-form-item field="config.community" label="Community 社区字符串">
              <a-input v-model="dialog.form.config.community" placeholder="public" />
            </a-form-item>
          </div>

          <div v-if="dialog.form.config.snmpVersion === 'v3'">
            <div class="modal-section__title modal-section__title--sub">SNMP v3 安全</div>
            <a-form-item field="config.securityName" label="安全名称 (用户名)" required>
              <a-input v-model="dialog.form.config.securityName" placeholder="admin" />
            </a-form-item>
            <a-row :gutter="[24, 16]" class="field-grid">
              <a-col :span="8">
                <a-form-item field="config.securityLevel" label="安全级别">
                  <a-select v-model="dialog.form.config.securityLevel" placeholder="authPriv">
                    <a-option value="noAuthNoPriv">noAuthNoPriv</a-option>
                    <a-option value="authNoPriv">authNoPriv</a-option>
                    <a-option value="authPriv">authPriv</a-option>
                  </a-select>
                </a-form-item>
              </a-col>
              <a-col :span="8">
                <a-form-item field="config.authProtocol" label="认证协议">
                  <a-select v-model="dialog.form.config.authProtocol" placeholder="SHA256">
                    <a-option value="MD5">MD5</a-option>
                    <a-option value="SHA1">SHA1</a-option>
                    <a-option value="SHA224">SHA224</a-option>
                    <a-option value="SHA256">SHA256</a-option>
                    <a-option value="SHA384">SHA384</a-option>
                    <a-option value="SHA512">SHA512</a-option>
                  </a-select>
                </a-form-item>
              </a-col>
              <a-col :span="8">
                <a-form-item field="config.privProtocol" label="加密协议">
                  <a-select v-model="dialog.form.config.privProtocol" placeholder="AES128">
                    <a-option value="DES">DES</a-option>
                    <a-option value="AES128">AES128</a-option>
                    <a-option value="AES192">AES192</a-option>
                    <a-option value="AES256">AES256</a-option>
                  </a-select>
                </a-form-item>
              </a-col>
            </a-row>
            <a-row :gutter="[24, 16]" class="field-grid">
              <a-col :span="12">
                <a-form-item field="config.authPassword" label="认证密码">
                  <a-input-password v-model="dialog.form.config.authPassword" placeholder="AuthPass123" />
                </a-form-item>
              </a-col>
              <a-col :span="12">
                <a-form-item field="config.privPassword" label="加密密码">
                  <a-input-password v-model="dialog.form.config.privPassword" placeholder="PrivPass123" />
                </a-form-item>
              </a-col>
            </a-row>
            <a-row :gutter="[24, 16]" class="field-grid">
              <a-col :span="12">
                <a-form-item field="config.contextName" label="上下文名称">
                  <a-input v-model="dialog.form.config.contextName" placeholder="可选" />
                </a-form-item>
              </a-col>
              <a-col :span="12">
                <a-form-item field="config.contextEngineID" label="上下文 Engine ID">
                  <a-input v-model="dialog.form.config.contextEngineID" placeholder="可选" />
                </a-form-item>
              </a-col>
            </a-row>
          </div>
        </div>

        <!-- EtherNet/IP Config -->
        <div v-if="dialog.form.protocol === 'ethernet-ip'" class="config-section">
          <div class="modal-section__title modal-section__title--sub">基础连接</div>
          <a-form-item field="config.ip" label="PLC IP 地址" required>
            <a-input v-model="dialog.form.config.ip" placeholder="192.168.1.10" />
          </a-form-item>
          <a-form-item field="config.port" label="PLC 端口">
            <a-input-number v-model="dialog.form.config.port" :min="1" :max="65535" placeholder="44818 (默认)" />
          </a-form-item>
          <a-form-item field="config.slot" label="槽号 (Slot)">
            <a-input-number v-model="dialog.form.config.slot" :min="0" :max="10" placeholder="0 (默认)" />
          </a-form-item>
          <a-form-item field="config.connection_type" label="连接类型">
            <a-select v-model="dialog.form.config.connection_type" placeholder="请选择连接类型">
              <a-option value="cip">标准 CIP 模式</a-option>
              <a-option value="logix">Logix 模式</a-option>
            </a-select>
          </a-form-item>

          <div class="modal-section__title modal-section__title--sub">通信参数</div>
          <a-row :gutter="[24, 16]" class="field-grid">
            <a-col :span="8">
              <a-form-item field="config.timeout" label="超时时间 (ms)">
                <a-input-number v-model="dialog.form.config.timeout" :min="500" :max="30000" placeholder="2000" />
              </a-form-item>
            </a-col>
            <a-col :span="8">
              <a-form-item field="config.max_retries" label="重试次数">
                <a-input-number v-model="dialog.form.config.max_retries" :min="0" :max="10" placeholder="3" />
              </a-form-item>
            </a-col>
            <a-col :span="8">
              <a-form-item field="config.retry_interval" label="重试间隔 (ms)">
                <a-input-number v-model="dialog.form.config.retry_interval" :min="10" :max="5000" placeholder="100" />
              </a-form-item>
            </a-col>
          </a-row>
          <a-row :gutter="[24, 16]" class="field-grid">
            <a-col :span="8">
              <a-form-item field="config.heartbeat_interval" label="心跳间隔 (ms)">
                <a-input-number v-model="dialog.form.config.heartbeat_interval" :min="0" :max="300000" placeholder="30000" />
              </a-form-item>
            </a-col>
            <a-col :span="8">
              <a-form-item field="config.batch_read_max" label="批量读取上限">
                <a-input-number v-model="dialog.form.config.batch_read_max" :min="1" :max="200" placeholder="50" />
              </a-form-item>
            </a-col>
            <a-col :span="8">
              <a-form-item field="config.min_interval" label="最小间隔 (ms)">
                <a-input-number v-model="dialog.form.config.min_interval" :min="0" :max="1000" placeholder="5" />
              </a-form-item>
            </a-col>
          </a-row>
        </div>

        <!-- EtherCAT 通道配置 -->
        <div v-if="dialog.form.protocol === 'ethercat'" class="config-section">
          <div class="modal-section__title modal-section__title--sub">基础连接</div>
          <a-form-item field="config.local_interface" label="网络接口" required>
            <a-input v-model="dialog.form.config.local_interface" placeholder="eth0 或 lo (模拟)" />
            <template #extra>
              物理网卡名称（如 eth0、enp2s0）或 lo（模拟模式）
            </template>
          </a-form-item>
          <a-row :gutter="[24, 16]" class="field-grid">
            <a-col :span="8">
              <a-form-item field="config.cycle_time_us" label="周期时间 (μs)">
                <a-input-number v-model="dialog.form.config.cycle_time_us" :min="100" :max="100000" placeholder="1000" />
              </a-form-item>
            </a-col>
            <a-col :span="8">
              <a-form-item field="config.timeout" label="超时时间 (ms)">
                <a-input-number v-model="dialog.form.config.timeout" :min="500" :max="60000" placeholder="3000" />
              </a-form-item>
            </a-col>
            <a-col :span="8">
              <a-form-item field="config.max_retries" label="重试次数">
                <a-input-number v-model="dialog.form.config.max_retries" :min="0" :max="10" placeholder="3" />
              </a-form-item>
            </a-col>
          </a-row>
          <a-form-item field="config.simulation" label="模拟模式">
            <a-switch v-model="dialog.form.config.simulation" />
            <template #extra>
              开启后使用内置模拟器，无需真实 EtherCAT 硬件即可测试
            </template>
          </a-form-item>
        </div>
      </a-form>
    </a-modal>

    <!-- Channel Metrics Modal -->
    <a-modal
      v-model:visible="metricsDialog.show"
      :width="800"
      modal-class="channel-metrics-modal"
      unmount-on-close
    >
      <template #title>
        <div class="channel-metrics-modal__title">
          <span class="protocol-tag" v-if="metricsDialog.channel?.protocol">
            {{ formatProtocolTag(metricsDialog.channel.protocol) }}
          </span>
          <span class="channel-metrics-modal__title-text">通道监控 · {{ metricsDialog.channel?.name || '' }}</span>
        </div>
      </template>

      <ChannelMetricsPanel
        :loading="metricsDialog.loading"
        :error="metricsDialog.error"
        :metrics="metricsDialog.metrics"
      />

      <template #footer>
        <a-button type="primary" @click="metricsDialog.show = false">关闭</a-button>
      </template>
    </a-modal>

    <ChannelProtocolHelpDrawer
      v-model:visible="channelHelpVisible"
      :initial-protocol="helpProtocol"
    />

    <a-modal
      v-model:visible="deleteDialog.visible"
      title="确认删除"
      ok-text="确认删除"
      cancel-text="取消"
      :ok-button-props="{ status: 'danger' }"
      @ok="executeDeleteChannel"
      @cancel="deleteDialog.visible = false"
    >
      <p>确定要删除通道 <strong>{{ deleteDialog.channel?.name }}</strong> 吗？</p>
      <p class="text-secondary">此操作不可撤销。</p>
    </a-modal>

    <a-modal
      v-model:visible="batchDeleteDialog.visible"
      title="批量删除通道"
      ok-text="确认删除"
      cancel-text="取消"
      :ok-loading="batchDeleteDialog.loading"
      :ok-button-props="{ status: 'danger' }"
      @ok="executeBatchDelete"
      @cancel="batchDeleteDialog.visible = false"
    >
      <p>确定要删除选中的 <strong>{{ selectedChannels.length }}</strong> 个通道吗？</p>
      <p class="text-secondary">此操作不可撤销。</p>
    </a-modal>
  </div>
</template>

<script setup>
import { ref, onMounted, reactive, computed, watch } from 'vue'
import { useRouter } from 'vue-router'
import { Message } from '@arco-design/web-vue'
import request from '@/utils/request'
import { formatProtocolTag } from '@/utils/protocolLabel'
import { applyChannelDefaultConfig, getChannelDefaultConfig } from '@/utils/channelDefaultConfig'
import ChannelProtocolHelpDrawer from '@/components/channel-help/ChannelProtocolHelpDrawer.vue'
import ChannelMetricsPanel from '@/components/channel/ChannelMetricsPanel.vue'
import { computeQualityScore, runtimeFromQualityScore } from '@/utils/channelMetrics'
import {
  IconApps, IconLink, IconSettings, IconTool, IconArrowRight,
  IconDown, IconCheckCircle, IconMinusCircle, IconDelete
} from '@arco-design/web-vue/es/icon'

const router = useRouter()
const loading = ref(false)
const viewMode = ref('card')
const selectionMode = ref(false)
const selectedChannels = ref([])
const channels = ref([])
let channelsFetchSeq = 0

const dialog = reactive({
  show: false,
  isEdit: false,
  form: {
    id: '',
    name: '',
    protocol: 'modbus-tcp',
    enable: true,
    config: {},
    devices: []
  }
})

const metricsDialog = reactive({
  show: false,
  loading: false,
  error: null,
  channel: null,
  metrics: null,
})

const channelHelpVisible = ref(false)

const helpProtocol = computed(() => dialog.form.protocol || 'modbus-tcp')

const openChannelHelp = () => {
  channelHelpVisible.value = true
}

const protocols = [
  { label: 'Modbus TCP', value: 'modbus-tcp' },
  { label: 'Modbus RTU Over TCP', value: 'modbus-rtu-over-tcp' },
  { label: 'Modbus RTU', value: 'modbus-rtu' },
  { label: 'BACnet IP', value: 'bacnet-ip' },
  { label: 'OPC UA', value: 'opc-ua' },
  { label: 'S7', value: 's7' },
  { label: 'DLT645', value: 'dlt645' },
  { label: 'EtherNet/IP', value: 'ethernet-ip' },
  { label: 'Omron FINS', value: 'omron-fins' },
  { label: 'KNXnet/IP', value: 'knxnet-ip' },
  { label: 'Profinet IO', value: 'profinet-io' },
  { label: 'Mitsubishi MC', value: 'mitsubishi-slmp' },
  { label: 'IEC 60870-5-104', value: 'iec60870-5-104' },
  { label: 'SNMP', value: 'snmp' },
  { label: 'EtherCAT', value: 'ethercat' }
]

const tableColumns = [
  { title: '通道名称', slotName: 'name', width: 200 },
  { title: '协议类型', dataIndex: 'protocol', width: 140, render: ({ record }) => formatProtocolTag(record?.protocol) },
  { title: '启用状态', slotName: 'enable', width: 108 },
  { title: '运行状态', slotName: 'runtime', width: 120 },
  { title: '关联设备', slotName: 'deviceCount', width: 72, align: 'center' },
  { title: '操作', slotName: 'actions', width: 220, fixed: 'right' },
]

const rowSelection = {
  selectedRowKeys: selectedChannels,
  onChange: (keys) => {
    selectedChannels.value = keys
  }
}

const enabledChannels = computed(() =>
  channels.value.filter(ch => ch.enable)
)

const disabledChannels = computed(() =>
  channels.value.filter(ch => !ch.enable)
)

const channelZones = computed(() => [
  { key: 'enabled', title: '已启用', items: enabledChannels.value },
  { key: 'disabled', title: '已禁用', items: disabledChannels.value },
])

const getProtocolClass = (protocol) => {
  const classes = {
    'modbus-tcp': 'protocol-tcp',
    'modbus-rtu': 'protocol-rtu',
    'modbus-rtu-over-tcp': 'protocol-tcp',
    'bacnet-ip': 'protocol-bacnet',
    'opc-ua': 'protocol-opc',
    's7': 'protocol-s7',
    'profinet-io': 'protocol-profinet-io',
    'ethernet-ip': 'protocol-ip',
    'mitsubishi-slmp': 'protocol-mitsubishi',
    'omron-fins': 'protocol-omron',
    'ethercat': 'protocol-ethercat'
  }
  return classes[protocol] || 'protocol-default'
}

const getQualityClass = (score) => {
  if (score === undefined || score === null || score === 0) return 'quality-none'
  if (score === 100) return 'quality-perfect'
  if (score >= 90) return 'quality-good'
  if (score >= 80) return 'quality-fair'
  return 'quality-poor'
}

const getQualityBarClass = (score) => {
  if (score === undefined || score === null || score === 0) return 'bar-none'
  if (score === 100) return 'bar-perfect'
  if (score >= 90) return 'bar-good'
  if (score >= 80) return 'bar-fair'
  return 'bar-poor'
}

const getSuccessRateClass = (rate) => {
  if (!rate && rate !== 0) return ''
  if (rate >= 0.99) return 'success'
  if (rate >= 0.95) return 'warning'
  return 'error'
}

const formatPercent = (val) => {
  if (val === undefined || val === null) return '-'
  return (val * 100).toFixed(0) + '%'
}

const formatCardRuntimeText = (text) => {
  if (!text) return '-'
  const idx = text.indexOf(' (')
  return idx >= 0 ? text.slice(0, idx) : text
}

const formatDuration = (ms) => {
  if (ms === undefined || ms === null) return '-'
  if (ms < 1) return '<1ms'
  if (ms < 1000) return ms.toFixed(2) + 'ms'
  return (ms / 1000).toFixed(2) + 's'
}

const isSelected = (id) => selectedChannels.value.includes(id)

const toggleSelectionMode = () => {
  selectionMode.value = !selectionMode.value
  selectedChannels.value = []
}

const toggleChannelSelection = (id) => {
  const idx = selectedChannels.value.indexOf(id)
  if (idx === -1) {
    selectedChannels.value.push(id)
  } else {
    selectedChannels.value.splice(idx, 1)
  }
}

const handleCardClick = (channel) => {
  if (selectionMode.value) {
    toggleChannelSelection(channel.id)
  } else {
    goToDevices(channel)
  }
}

const goToDevices = (channel) => {
  router.push(`/channels/${channel.id}/devices`)
}

const fillDefaultConfig = () => {
  const protocol = dialog.form.protocol
  if (!protocol) return
  dialog.form.config = applyChannelDefaultConfig(protocol, dialog.form.config)
  Message.success('已填充推荐参数（仅空字段）')
}

const openAddDialog = () => {
  dialog.isEdit = false
  dialog.form = {
    id: '',
    name: '',
    protocol: 'modbus-tcp',
    enable: true,
    config: getChannelDefaultConfig('modbus-tcp'),
    devices: []
  }
  dialog.show = true
}

const openEditDialog = (channel) => {
  dialog.isEdit = true
  dialog.form = { ...JSON.parse(JSON.stringify(channel)), config: { ...(channel.config || {}) } }
  dialog.show = true
}

const generateId = () => {
  if (dialog.isEdit) return
  const chars = 'abcdefghijklmnopqrstuvwxyz0123456789'
  let result = ''
  for (let i = 0; i < 16; i++) {
    result += chars.charAt(Math.floor(Math.random() * chars.length))
  }
  dialog.form.id = result
}

const saveChannel = async () => {
  try {
    if (!dialog.form.id || !dialog.form.name) {
      Message.error('请填写完整信息')
      return false
    }

    if (dialog.form.protocol === 'opc-ua' && dialog.form.config?.url) {
      dialog.form.config.endpoint = dialog.form.config.url
    }
    
    if (dialog.isEdit) {
      await request({
        url: `/api/channels/${dialog.form.id}`,
        method: 'put',
        data: dialog.form
      })
      Message.success('通道更新成功')
    } else {
      await request({
        url: '/api/channels',
        method: 'post',
        data: dialog.form
      })
      Message.success('通道添加成功')
    }

    await fetchChannels()
    return true
  } catch (e) {
    Message.error('操作失败: ' + e.message)
    return false
  }
}

const deleteDialog = reactive({
  visible: false,
  channel: null
})

const deleteChannel = (channel) => {
  deleteDialog.channel = channel
  deleteDialog.visible = true
}

const executeDeleteChannel = async () => {
  const channel = deleteDialog.channel
  if (!channel) return

  try {
    await request({
      url: `/api/channels/${channel.id}`,
      method: 'delete'
    })
    Message.success('通道删除成功')
    deleteDialog.visible = false
    deleteDialog.channel = null
    fetchChannels()
  } catch (e) {
    Message.error('删除失败: ' + e.message)
  }
}

const openMetricsDialog = async (channel) => {
  metricsDialog.channel = channel
  metricsDialog.error = null
  metricsDialog.metrics = null
  metricsDialog.loading = true
  metricsDialog.show = true

  try {
    const timeoutPromise = new Promise((_, reject) =>
      setTimeout(() => reject(new Error('加载超时')), 3000)
    )

    const metricsRes = await Promise.race([
      request({
        url: `/api/channels/${channel.id}/metrics`,
        method: 'get',
      }),
      timeoutPromise,
    ])
    metricsDialog.metrics = metricsRes
  } catch (error) {
    metricsDialog.error = `获取监控指标失败: ${error.message}`
    console.error('Failed to get channel metrics:', error)
  } finally {
    metricsDialog.loading = false
  }
}

const scanChannel = (channel) => {
  // Navigate to device list page where device scanning is implemented
  router.push(`/channels/${channel.id}/devices`)
}

const batchConfigLoading = ref(false)

const onBatchConfigSelect = async (key) => {
  if (key === 'enable' || key === 'disable') {
    const enable = key === 'enable'
    batchConfigLoading.value = true
    try {
      await Promise.all(selectedChannels.value.map(id => request.put(`/api/channels/${id}`, { enable })))
      Message.success(`已批量${enable ? '启用' : '禁用'} ${selectedChannels.value.length} 个通道`)
      selectedChannels.value = []
      if (selectionMode.value) toggleSelectionMode()
      await fetchChannels()
    } catch (e) {
      Message.error('批量更新失败: ' + e.message)
    } finally {
      batchConfigLoading.value = false
    }
  } else if (key === 'delete') {
    batchDeleteDialog.visible = true
  }
}

const batchDeleteDialog = reactive({
  visible: false,
  loading: false
})

const executeBatchDelete = async () => {
  if (batchDeleteDialog.loading) return
  batchDeleteDialog.loading = true
  try {
    await Promise.all(selectedChannels.value.map(id => request({
      url: `/api/channels/${id}`,
      method: 'delete'
    })))
    Message.success(`已删除 ${selectedChannels.value.length} 个通道`)
    selectedChannels.value = []
    batchDeleteDialog.visible = false
    if (selectionMode.value) toggleSelectionMode()
    await fetchChannels()
  } catch (e) {
    Message.error('批量删除失败: ' + e.message)
  } finally {
    batchDeleteDialog.loading = false
  }
}

const getRuntimeColor = (state) => {
  const colorMap = {
    'running': 'success',
    'error': 'danger',
    'offline': 'gray'
  }
  return colorMap[state] || 'gray'
}

const getRuntimeText = (state) => {
  const textMap = {
    'running': '运行中',
    'error': '错误',
    'offline': '离线'
  }
  return textMap[state] || '未知'
}

const fetchChannels = async () => {
  const seq = ++channelsFetchSeq
  loading.value = true
  try {
    const res = await request({ url: '/api/channels', method: 'get' })
    if (seq !== channelsFetchSeq) return

    const rawData = Array.isArray(res) ? res : (res.data || [])
    
    // 第一步：快速展示通道列表（不等待指标）
    const channelsWithoutMetrics = rawData.map((item) => {
      const count = Array.isArray(item.devices) ? item.devices.length : 0
      const enableText = item.enable ? '已启用' : '已禁用'
      const enableColor = item.enable ? 'green' : 'gray'
      
      // 初始状态：如果enabled则默认在线，否则离线
      let runtimeText = '离线'
      let runtimeArcoStatus = 'normal'
      
      if (item.enable) {
        runtimeText = '运行中'
        runtimeArcoStatus = 'success'
      }
      
      return {
        ...item,
        deviceCount: count,
        enableText,
        enableColor,
        runtimeText,
        runtimeArcoStatus,
        qualityScore: null,
        successRate: null,
        metrics: null
      }
    })
    
    channels.value = channelsWithoutMetrics
    loading.value = false // 立即关闭loading
    
    // 第二步：异步加载指标数据
    Promise.all(
      rawData.map(async (item) => {
        try {
          const metricsRes = await request({
            url: `/api/channels/${item.id}/metrics`,
            method: 'get'
          })
          return { channelId: item.id, metrics: metricsRes }
        } catch (metricsError) {
          console.warn(`获取通道 ${item.id} 指标失败:`, metricsError)
          return { channelId: item.id, metrics: null }
        }
      })
    ).then((metricsResults) => {
      if (seq !== channelsFetchSeq) return

      metricsResults.forEach((result) => {
        const channelIndex = channels.value.findIndex(ch => ch.id === result.channelId)
        if (channelIndex >= 0) {
          const metrics = result.metrics
          
          if (metrics) {
            const score = computeQualityScore(metrics)
            const runtime = runtimeFromQualityScore(score, metrics)

            channels.value[channelIndex] = {
              ...channels.value[channelIndex],
              metrics,
              qualityScore: score,
              successRate: metrics.successRate ?? metrics.success_rate ?? null,
              runtimeText: runtime.text,
              runtimeArcoStatus: runtime.status,
            }
          }
        }
      })
    }).catch((err) => {
      console.error('批量加载指标失败:', err)
    })
  } catch (e) {
    if (seq !== channelsFetchSeq) return
    Message.error('加载通道列表失败: ' + e.message)
    loading.value = false
  }
}

watch(
  () => dialog.form.protocol,
  (protocol, prev) => {
    if (!dialog.show || dialog.isEdit || !protocol || protocol === prev) return
    // Adding a channel: switching protocol replaces config with that protocol's defaults.
    dialog.form.config = getChannelDefaultConfig(protocol)
  }
)

onMounted(() => {
  fetchChannels()
})
</script>

<style scoped>
/* v3.0 — styles in src/styles/channel-list.css */
</style>