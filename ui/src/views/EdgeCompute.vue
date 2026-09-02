<template>
  <div class="page-shell page-shell--wide edge-compute-container edge-compute-page">
    <div class="page-header">
      <div>
        <h2 class="page-title">边缘计算</h2>
        <p class="page-subtitle">规则监控、管理与运行日志</p>
      </div>
    </div>

    <a-tabs v-model:active-key="tab" type="rounded" size="small" class="main-tabs">
      <a-tab-pane key="metrics" title="监控面板" />
      <a-tab-pane key="rules" title="规则管理" />
      <a-tab-pane key="templates" title="场景模版" />
      <a-tab-pane key="status" title="记录与日志" />
    </a-tabs>

    <div class="edge-compute-body">
      <template v-if="tab === 'metrics'">
        <EdgeComputeMetrics />
        <EdgeComputePageFooter
          @switch-tab="(key) => tab = key"
          @open-help="helpVisible = true"
        />
      </template>

      <div v-if="tab === 'rules'" class="edge-compute-flow">
        <section class="edge-compute-panel" aria-label="规则管理">
          <div class="edge-compute-toolbar edge-compute-toolbar--view">
            <div class="edge-compute-toolbar__left">
              <a-space size="small">
                <a-button type="primary" size="small" @click="openDialog">
                  <template #icon><IconPlus /></template>
                  添加规则
                </a-button>
                <a-button type="outline" size="small" @click="openJsonImport">
                  <template #icon><IconUpload /></template>
                  JSON导入
                </a-button>
                <a-button type="text" size="small" class="help-trigger-btn" @click="helpVisible = true">
                  <template #icon><IconQuestionCircle /></template>
                  帮助说明
                </a-button>
              </a-space>
              <span v-if="rules.length > 0" class="edge-compute-panel-meta">{{ rules.length }} 条规则</span>
            </div>
            <div class="edge-compute-toolbar__right">
              <a-radio-group v-model="rulesViewMode" type="button" size="small" class="edge-compute-view-toggle">
                <a-radio value="flow">流程视图</a-radio>
                <a-radio value="table">表格视图</a-radio>
              </a-radio-group>
            </div>
          </div>
          <div v-show="selectedRuleKeys.length > 0" class="edge-compute-toolbar edge-compute-toolbar--batch">
            <span class="selection-count">已选 {{ selectedRuleKeys.length }} 项</span>
            <a-divider direction="vertical" />
            <a-button size="small" type="outline" @click="handleBatchEnable(true)">批量启用</a-button>
            <a-button size="small" type="outline" @click="handleBatchEnable(false)">批量禁用</a-button>
            <a-button size="small" type="outline" status="danger" @click="handleBatchDelete">批量删除</a-button>
          </div>
          <div class="edge-compute-tertiary-block">
            <div v-if="rulesViewMode === 'flow'" class="rule-flow-list">
              <RuleFlowCard
                v-for="rule in rules"
                :key="rule.id"
                :rule="rule"
                :state="ruleStateMap[rule.id]"
                :selected="selectedRuleKeys.includes(rule.id)"
                :compact="true"
                @select="(checked) => toggleRuleSelection(rule.id, checked)"
              >
                <template #operations>
                  <span class="table-ops">
                    <a-button
                      v-if="rule.type === 'window'"
                      type="text"
                      size="small"
                      @click="viewWindowData(rule.id, rule.name)"
                    >
                      窗口数据
                    </a-button>
                    <a-button type="text" size="small" @click="editRule(rule)">
                      <template #icon><IconEdit /></template>
                    </a-button>
                    <a-button type="text" size="small" status="danger" @click="deleteRule(rule)">
                      <template #icon><IconDelete /></template>
                    </a-button>
                  </span>
                </template>
              </RuleFlowCard>
              <a-empty v-if="rules.length === 0" class="empty-wrap">
                <template #image><IconStorage :size="48" class="empty-icon-muted" /></template>
                <div class="empty-title">暂无规则</div>
                <div class="empty-desc">点击「添加规则」创建第一条边缘计算规则</div>
              </a-empty>
            </div>
            <div v-else class="table-container saas-table">
              <a-table
                :columns="ruleColumns"
                :data="rules"
                size="small"
                :bordered="false"
                :scroll="{ x: 960 }"
                :row-selection="{ type: 'checkbox', showCheckedAll: true }"
                v-model:selected-keys="selectedRuleKeys"
                row-key="id"
              >
                <template #operations="{ record }">
                  <span class="table-ops">
                    <a-button type="text" size="small" @click="editRule(record)">
                      <template #icon><IconEdit /></template>
                    </a-button>
                    <a-button type="text" size="small" status="danger" @click="deleteRule(record)">
                      <template #icon><IconDelete /></template>
                    </a-button>
                  </span>
                </template>
                <template #empty>
                  <a-empty description="暂无规则">
                    <template #image><IconStorage :size="48" class="empty-icon-muted" /></template>
                    <div class="empty-title">暂无规则</div>
                    <div class="empty-desc">点击「添加规则」创建第一条边缘计算规则</div>
                  </a-empty>
                </template>
                <template #enable="{ record }">
                  <span class="table-cell-semantic">
                    <a-tag :color="record.enable ? 'success' : 'danger'" size="small">
                      {{ record.enable ? '启用' : '禁用' }}
                    </a-tag>
                  </span>
                </template>
                <template #type="{ record }">
                  {{ formatRuleType(record.type) }}
                </template>
                <template #trigger_mode="{ record }">
                  {{ formatTriggerMode(record.trigger_mode) }}
                </template>
              </a-table>
            </div>
          </div>
        </section>
      </div>

      <EdgeSceneTemplates
        v-if="tab === 'templates'"
        @apply="applyRuleTemplate"
      />

      <div v-if="tab === 'status'" class="edge-compute-flow">
        <section class="edge-compute-panel" aria-label="记录与日志">
          <div class="edge-compute-toolbar edge-compute-toolbar--records">
            <div class="edge-compute-toolbar__left edge-compute-records-heading">
              <a-radio-group v-model="recordType" type="button" size="small" class="edge-compute-record-type-toggle">
                <a-radio value="events">错误事件</a-radio>
                <a-radio value="logs">错误日志</a-radio>
              </a-radio-group>
              <a-tooltip v-if="recordType === 'events'" content="仅记录公式匹配、执行、调度等异常事件；正常触发与校验不产生日志。">
                <IconInfoCircle class="edge-compute-records-info" />
              </a-tooltip>
              <span v-if="recordCount > 0" class="edge-compute-panel-meta">{{ recordCount }} 条</span>
            </div>
            <div class="edge-compute-toolbar__right">
              <a-space size="small">
                <a-badge :count="activeFilterCount" :dot="activeFilterCount > 0">
                  <a-button type="outline" size="small" @click="filterVisible = true">
                    <template #icon><IconFilter /></template>
                    筛选
                  </a-button>
                </a-badge>
                <a-button type="outline" size="small" @click="refreshRecords">
                  <template #icon><IconRefresh /></template>
                  刷新
                </a-button>
                <a-popconfirm
                  content="确定清空所有边缘计算错误日志（错误事件、失败记录、分钟级错误日志）？规则与实时状态不会受影响。"
                  @ok="clearEdgeLogs"
                >
                  <a-button type="outline" status="danger" size="small">
                    清空日志
                  </a-button>
                </a-popconfirm>
                <a-button
                  v-if="recordType === 'logs'"
                  type="outline"
                  size="small"
                  @click="exportLogs"
                  :disabled="logs.length === 0"
                >
                  <template #icon><IconDownload /></template>
                  导出 CSV
                </a-button>
              </a-space>
            </div>
          </div>

          <div v-if="activeFilterChips.length > 0" class="edge-compute-filter-chips">
            <a-tag
              v-for="chip in activeFilterChips"
              :key="chip.key"
              size="small"
              closable
              @close="removeFilterChip(chip.key)"
            >
              {{ chip.label }}
            </a-tag>
            <a-button type="text" size="mini" @click="resetRecordFilters">清除筛选</a-button>
          </div>

          <template v-if="recordType === 'events'">
            <a-empty v-if="edgeEvents.length === 0" class="empty-wrap">
              <template #image><IconHistory :size="48" class="empty-icon-muted" /></template>
              <div class="empty-title">暂无错误事件</div>
              <div class="empty-desc">规则发生公式匹配、执行或调度异常时将在此显示</div>
            </a-empty>
            <div v-else class="edge-compute-tertiary-block">
              <div class="table-container saas-table edge-compute-records-table">
                <a-table
                  :columns="eventColumns"
                  :data="edgeEvents"
                  size="small"
                  :bordered="false"
                  :pagination="{ pageSize: 20, showTotal: true }"
                  row-key="id"
                >
                  <template #status="{ record }">
                    <a-tag :color="getEventStatusColor(record.status)" size="small">{{ record.status }}</a-tag>
                  </template>
                  <template #duration_ms="{ record }">
                    {{ record.duration_ms != null ? `${record.duration_ms} ms` : '—' }}
                  </template>
                  <template #started_at="{ record }">
                    {{ formatDate(record.started_at) }}
                  </template>
                  <template #actions="{ record }">
                    <span class="single-line-cell" :title="formatEventActions(record.actions)">
                      {{ formatEventActions(record.actions) }}
                    </span>
                  </template>
                  <template #error_message="{ record }">
                    <span v-if="record.error_message" class="edge-compute-records-error">{{ record.error_message }}</span>
                    <span v-else class="edge-compute-records-muted">—</span>
                  </template>
                </a-table>
              </div>
            </div>
          </template>

          <template v-else>
            <a-empty v-if="logs.length === 0" class="empty-wrap">
              <template #image><IconClockCircle :size="48" class="empty-icon-muted" /></template>
              <div class="empty-title">暂无错误日志</div>
              <div class="empty-desc">规则运行出错后将按分钟汇总错误信息</div>
            </a-empty>
            <div v-else class="edge-compute-tertiary-block">
              <div class="table-container saas-table">
                <a-table
                  :columns="logColumns"
                  :data="logs"
                  size="small"
                  :bordered="false"
                  :scroll="{ x: 1200 }"
                  :pagination="{ pageSize: 20, showTotal: true }"
                >
                  <template #error_type="{ record }">
                    <a-tag color="red" size="small">{{ formatErrorType(record.error_type) }}</a-tag>
                  </template>
                  <template #error_message="{ record }">
                    <span class="single-line-cell text-error" @click="showDetails('错误详情', record.error_message)" title="点击查看详情">
                      {{ record.error_message || '—' }}
                    </span>
                  </template>
                </a-table>
              </div>
            </div>
          </template>
        </section>
      </div>
    </div>

    <!-- Window Data Dialog -->
    <a-modal v-model:visible="windowDialog" title="窗口数据预览 ({{ currentWindowRuleName }})" width="600px">
      <a-table
        :columns="windowDataColumns"
        :data="windowData"
        size="small"
        :bordered="false"
      >
        <template #ts="{ record }">
          {{ formatDate(record.ts) }}
        </template>
      </a-table>
      <template #footer>
        <a-button type="primary" @click="windowDialog = false">关闭</a-button>
      </template>
    </a-modal>

    <!-- Details Dialog -->
    <a-modal v-model:visible="detailsDialog" :title="detailTitle" width="800px">
      <a-tabs v-model:active-key="detailsTab" class="mb-4">
        <a-tab-pane key="text" title="文本/原始内容"></a-tab-pane>
        <a-tab-pane key="hex" title="Hex 视图" :disabled="!decodedHex"></a-tab-pane>
      </a-tabs>
            
      <div v-if="detailsTab === 'text'">
        <div class="text-gray-500 mb-2">内容长度: {{ detailContent.length }}</div>
        <a-textarea
          v-model="detailContent"
          readonly
          :auto-size="{ minRows: 5, maxRows: 15 }"
          style="font-family: monospace;"
        />
      </div>
            
      <div v-else-if="detailsTab === 'hex'">
        <div class="text-gray-500 mb-2">Hex 视图 ({{ decodedBytes ? decodedBytes.length : 0 }} bytes)</div>
        <a-textarea
          v-model="decodedHex"
          readonly
          :auto-size="{ minRows: 5, maxRows: 15 }"
          style="font-family: monospace;"
        />
      </div>

      <a-alert v-if="detectedFile" type="info" class="mt-4">
        <div class="flex justify-between items-center">
          <span>检测到文件格式: <strong>{{ detectedFile.name }} ({{ detectedFile.ext }})</strong></span>
          <a-button type="primary" size="small" @click="downloadDetectedFile">
            <template #icon><IconDownload /></template>
            下载文件
          </a-button>
        </div>
      </a-alert>
      <template #footer>
        <a-button v-if="!decodedHex" @click="tryDecode">
          <template #icon><IconCode /></template>
          尝试 Base64 解码
        </a-button>
        <a-button type="primary" @click="detailsDialog = false">关闭</a-button>
      </template>
    </a-modal>

    <!-- Rule Dialog -->
    <a-modal v-model:visible="dialog" :title="editingRule ? '编辑规则' : '添加规则'" width="80%" modal-class="industrial-white-modal edge-compute-rule-modal">
      <a-form ref="form" :model="currentRule" layout="vertical" class="industrial-form form-controls-md">
        <div class="form-section">
          <div class="section-title">基础配置</div>
          <a-row :gutter="16">
            <a-col :span="16">
              <a-form-item field="name" label="规则名称" required>
                <a-input v-model="currentRule.name" placeholder="请输入规则名称" class="rect-input" />
              </a-form-item>
            </a-col>
            <a-col :span="8">
              <a-form-item field="enable" label="启用状态">
                <a-switch v-model="currentRule.enable" type="round" />
              </a-form-item>
            </a-col>
          </a-row>
          <a-row :gutter="16">
            <a-col :span="6">
              <a-form-item field="type" label="规则类型">
                <a-select 
                  v-model="currentRule.type" 
                  class="rect-input"
                  :options="[
                    {label: 'Threshold (阈值触发)', value: 'threshold'},
                    {label: 'Calculation (计算公式)', value: 'calculation'},
                    {label: 'Window (时间/计数窗口)', value: 'window'},
                    {label: 'State (状态持续)', value: 'state'}
                  ]" 
                />
              </a-form-item>
            </a-col>
            <a-col :span="6">
              <a-form-item field="priority" label="优先级">
                <a-input-number v-model="currentRule.priority" class="rect-input" />
              </a-form-item>
            </a-col>
            <a-col :span="6">
              <a-form-item field="trigger_mode" label="触发模式">
                <a-select
                  v-model="currentRule.trigger_mode"
                  class="rect-input"
                  :options="[{label: '始终触发', value: 'always'}, {label: '仅状态改变时触发', value: 'on_change'}]"
                />
              </a-form-item>
            </a-col>
            <a-col :span="6">
              <a-form-item field="check_interval" label="检查频率">
                <a-select
                  v-model="currentRule.check_interval"
                  class="rect-input"
                  :options="['1s', '5s', '10s', '30s', '1m']"
                />
              </a-form-item>
            </a-col>
          </a-row>
          <div class="form-hint">{{ getRuleTypeExplanation(currentRule.type) }}</div>
        </div>

        <div class="form-section">
          <div class="section-header-row">
            <div class="section-title">数据源 Sources</div>
            <div class="flex gap-2">
              <a-button type="outline" size="small" @click="detectInvalidSources">
                <template #icon><IconRefresh /></template>
                自动检测
              </a-button>
              <a-button type="outline" status="danger" size="small" @click="clearInvalidSources">
                <template #icon><IconDelete /></template>
                一键清除
              </a-button>
              <a-button type="primary" size="small" @click="addSource">
                <template #icon><IconPlus /></template>
                添加
              </a-button>
            </div>
          </div>
          <div class="form-hint form-hint--block">
            请为每个数据源设置别名（如 t1, t2），然后在触发条件中使用别名编写逻辑公式（例如：t1 > 20 || t2 > 30）。
          </div>
          <div v-for="(src, index) in currentRule.sources" :key="index" class="source-row">
            <div class="source-index">#{{ index + 1 }}</div>
            <a-row :gutter="12" class="flex-1">
              <a-col :span="24" :md="5">
                <a-select
                  v-model="src.channel_id"
                  :options="channels"
                  placeholder="通道"
                  class="rect-input"
                  @change="() => onSourceChannelChange(src)"
                />
              </a-col>
              <a-col :span="24" :md="5">
                <a-select
                  v-model="src.device_id"
                  :options="src._deviceList || []"
                  placeholder="设备"
                  class="rect-input"
                  :disabled="!src.channel_id"
                  @change="() => onSourceDeviceChange(src)"
                  @click="() => loadSourceDevices(src)"
                />
              </a-col>
              <a-col :span="24" :md="5">
                <a-select
                  v-model="src.point_id"
                  :options="src._pointList || []"
                  placeholder="点位"
                  class="rect-input"
                  :disabled="!src.device_id"
                  @click="() => loadSourcePoints(src)"
                />
              </a-col>
              <a-col :span="24" :md="7">
                <a-input 
                  v-model="src.alias" 
                  placeholder="t1"
                  class="rect-input w-full"
                />
              </a-col>
              <a-col :span="24" :md="2" class="flex items-center justify-end">
                <a-button type="text" status="danger" @click="removeSource(index)">
                  <IconDelete />
                </a-button>
              </a-col>
            </a-row>
          </div>
        </div>

        <!-- 窗口配置 -->
        <div v-if="currentRule.type === 'window'" class="form-section">
          <div class="section-title">窗口配置</div>
          <a-row :gutter="16">
            <a-col :span="8">
              <a-form-item field="window.type" label="窗口类型">
                <a-select v-model="currentRule.window.type" :options="['sliding', 'tumbling']" class="rect-input" />
              </a-form-item>
            </a-col>
            <a-col :span="8">
              <a-form-item field="window.size" label="窗口大小">
                <a-input v-model="currentRule.window.size" placeholder="例如: 10s 或 100" class="rect-input" />
              </a-form-item>
            </a-col>
            <a-col :span="8">
              <a-form-item field="window.aggr_func" label="聚合函数">
                <a-select v-model="currentRule.window.aggr_func" :options="['avg', 'min', 'max', 'sum', 'count', 'rate']" class="rect-input" />
              </a-form-item>
            </a-col>
          </a-row>
        </div>

        <!-- 状态维持 -->
        <div v-if="currentRule.type === 'state' || currentRule.type === 'threshold'" class="form-section">
          <div class="section-title">状态维持</div>
          <a-row :gutter="16">
            <a-col :span="12">
              <a-form-item field="state.duration" label="持续时间 (Duration)">
                <a-input v-model="currentRule.state.duration" placeholder="例如: 10s" class="rect-input" />
              </a-form-item>
            </a-col>
            <a-col :span="12">
              <a-form-item field="state.count" label="连续次数 (Count)">
                <a-input-number v-model="currentRule.state.count" class="rect-input" />
              </a-form-item>
            </a-col>
          </a-row>
        </div>

        <!-- 规则逻辑 Logic -->
        <div v-if="currentRule.type !== 'calculation'" class="form-section">
          <div class="section-header-row">
            <div class="section-title">规则逻辑 Logic</div>
            <a-button size="small" @click="openHelper(currentRule.condition, (v) => currentRule.condition = v)">
              公式助手
            </a-button>
          </div>
          <a-form-item label="表达式">
            <a-textarea
              v-model="currentRule.condition"
              placeholder="t1 > 50 && t2 < 80"
              :rows="3"
              class="code-input rect-input"
            />
            <template #extra>支持数据源别名（如 t1, t2）和逻辑运算符</template>
          </a-form-item>
        </div>

        <!-- 计算公式 -->
        <div v-if="currentRule.type === 'calculation'" class="form-section">
          <div class="section-header-row">
            <div class="section-title">计算公式</div>
            <a-button size="small" @click="openHelper(currentRule.expression, (v) => currentRule.expression = v)">
              公式助手
            </a-button>
          </div>
          <a-form-item label="表达式">
            <a-textarea
              v-model="currentRule.expression"
              placeholder="value * 1.5 + 32"
              :rows="3"
              class="code-input rect-input"
            />
            <template #extra>支持数学运算符和函数（如 abs, sqrt, sin 等）</template>
          </a-form-item>
        </div>

        <!-- 动作 Actions -->
        <div class="form-section">
          <div class="section-header-row">
            <div class="section-title">执行动作 (Action)</div>
            <div class="flex gap-2">
              <a-button type="outline" size="small" @click="detectInvalidActions">
                <template #icon><IconRefresh /></template>
                自动检测
              </a-button>
              <a-button type="outline" status="danger" size="small" @click="clearInvalidActions">
                <template #icon><IconDelete /></template>
                一键清除
              </a-button>
              <a-button type="primary" size="small" @click="addAction">
                <template #icon><IconPlus /></template>
                添加动作
              </a-button>
            </div>
          </div>
          <div v-if="!currentRule.actions || currentRule.actions.length === 0" class="empty-box">
            无动作
          </div>
          <div v-else class="actions-list">
            <div v-for="(action, index) in currentRule.actions" :key="index" class="action-row">
              <div class="action-index">STEP {{ index + 1 }}</div>
              <ActionEditor 
                class="flex-1"
                v-model="currentRule.actions[index]" 
                :channels="channels" 
                @remove="removeAction(index)" 
              />
            </div>
          </div>
        </div>
      </a-form>
      <template #footer>
        <a-button @click="dialog = false">取消</a-button>
        <a-button type="primary" :loading="ruleSaving" @click="saveRule">保存</a-button>
      </template>
    </a-modal>

    <!-- JSON Import Dialog -->
    <a-modal v-model:visible="jsonImportVisible" title="JSON 导入规则" width="700px" :ok-text="jsonImportPreview.length > 0 ? `导入 ${jsonImportPreview.length} 条规则` : '导入'" :ok-loading="jsonImporting" @ok="executeJsonImport" :ok-button-props="{ disabled: jsonImportPreview.length === 0 }">
      <div class="json-import-area">
        <div class="json-import-tabs">
          <a-radio-group v-model="jsonImportMode" type="button" size="small">
            <a-radio value="paste">粘贴 JSON</a-radio>
            <a-radio value="file">上传文件</a-radio>
          </a-radio-group>
        </div>

        <div v-if="jsonImportMode === 'paste'" class="mt-3">
          <a-textarea
            v-model="jsonImportContent"
            placeholder="粘贴 JSON 规则内容，支持单条规则或规则数组。例如：
[
  {
    &quot;name&quot;: &quot;温度报警&quot;,
    &quot;type&quot;: &quot;threshold&quot;,
    &quot;enable&quot;: true,
    &quot;priority&quot;: 0,
    &quot;trigger_mode&quot;: &quot;always&quot;,
    &quot;check_interval&quot;: &quot;10s&quot;,
    &quot;sources&quot;: [{&quot;channel_id&quot;: &quot;...&quot;, &quot;device_id&quot;: &quot;...&quot;, &quot;point_id&quot;: &quot;...&quot;, &quot;alias&quot;: &quot;t1&quot;}],
    &quot;condition&quot;: &quot;t1 > 80&quot;,
    &quot;actions&quot;: [{&quot;type&quot;: &quot;mqtt&quot;, &quot;topic&quot;: &quot;alarm&quot;, &quot;payload&quot;: &quot;温度过高&quot;}]
  }
]"
            :rows="12"
            class="code-input rect-input"
            @input="onJsonInputChange"
          />
        </div>

        <div v-else class="mt-3">
          <div class="json-upload-area" @dragover.prevent @drop.prevent="handleJsonFileDrop">
            <a-upload
              :auto-upload="false"
              :show-file-list="false"
              accept=".json"
              @change="handleJsonFileSelect"
              draggable
            >
              <template #upload-button>
                <div class="json-upload-trigger">
                  <IconUpload :size="32" class="json-upload-icon" />
                  <div class="json-upload-text">点击或拖拽 JSON 文件到此处</div>
                  <div class="json-upload-hint">支持 .json 文件，单条规则或规则数组</div>
                </div>
              </template>
            </a-upload>
            <div v-if="jsonFileName" class="json-upload-file-info">
              <IconFile class="json-file-icon" />
              <span>{{ jsonFileName }}</span>
              <a-button type="text" size="mini" status="danger" @click="clearJsonFile">移除</a-button>
            </div>
          </div>
        </div>

        <div v-if="jsonImportError" class="mt-3">
          <a-alert type="error" :title="jsonImportError" closable @close="jsonImportError = ''" />
        </div>

        <div v-if="jsonImportPreview.length > 0" class="mt-4">
          <div class="json-import-preview-header">
            <span class="json-import-preview-title">预览 ({{ jsonImportPreview.length }} 条规则)</span>
            <a-button type="text" size="small" @click="clearJsonImport">清空</a-button>
          </div>
          <div class="json-import-preview-list">
            <div v-for="(rule, index) in jsonImportPreview" :key="index" class="json-import-preview-item">
              <span class="json-import-preview-index">#{{ index + 1 }}</span>
              <span class="json-import-preview-name">{{ rule.name || '(未命名)' }}</span>
              <a-tag :color="getRuleTypeTagColor(rule.type)" size="small">{{ formatRuleType(rule.type) }}</a-tag>
              <a-tag :color="rule.enable !== false ? 'green' : 'red'" size="small">{{ rule.enable !== false ? '启用' : '禁用' }}</a-tag>
              <span class="json-import-preview-source">{{ rule.sources?.length || 0 }} 个数据源</span>
              <span class="json-import-preview-action">{{ rule.actions?.length || 0 }} 个动作</span>
            </div>
          </div>
        </div>
      </div>
    </a-modal>

    <EdgeComputeHelpDrawer v-model:visible="helpVisible" />

    <EdgeRecordFilterModal
      v-model:visible="filterVisible"
      :mode="recordType"
      :filters="currentRecordFilters"
      :rules="rules"
      @apply="applyRecordFilters"
    />

    <!-- Expression Helper Dialog -->
    <a-modal v-model:visible="helperDialog" title="表达式转换助手" width="600px">
      <div class="pt-4">
        <div class="text-gray-500 mb-2">输入标准表达式 (例如: v & 64, v | 1, ~v):</div>
        <div class="text-gray-500 text-sm mb-4">提示: 系统已直接支持 v.N 语法 (如 v.4) 及 v.bit.N 语法 (如 v.bit.4) 读取第N位，无需转换。</div>
        <a-textarea
          v-model="helperInput"
          placeholder="标准表达式 (Standard Syntax)"
          :rows="3"
          :auto-size="{ minRows: 3, maxRows: 6 }"
        />
                        
        <div class="flex justify-center my-4 gap-4">
          <a-button type="text" @click="docsDialog = true">
            <template #icon><IconBook /></template>
            查看函数文档 (View Docs)
          </a-button>
          <a-button type="secondary" @click="convertHelper">
            <template #icon><IconArrowDown /></template>
            转换 (Convert)
          </a-button>
        </div>
                        
        <div class="text-gray-500 mb-2">转换结果 (Function Syntax):</div>
        <a-textarea
          v-model="helperOutput"
          placeholder="函数表达式 (Result)"
          :rows="3"
          :auto-size="{ minRows: 3, maxRows: 6 }"
          readonly
          :disabled="true"
        />
      </div>
      <template #footer>
        <a-button @click="helperDialog = false">关闭</a-button>
        <a-button type="primary" @click="applyHelper" :disabled="!helperOutput">应用并填入</a-button>
      </template>
    </a-modal>
    <!-- Expression Docs Dialog -->
    <a-modal v-model:visible="docsDialog" title="表达式函数参考文档 (Expression Reference)" width="950px" :scrollable="true" class="expression-docs-dialog">
      <div class="expression-docs-content" style="max-height: 650px; overflow-y: auto;">
        <a-alert type="info" class="mb-6 p-4">
          <div class="font-medium text-lg mb-2">基本变量</div>
          <div class="mb-1"><code class="text-blue-600">value</code> 或 <code class="text-blue-600">v</code> : 当前触发点位的值 (The current point value).</div>
          <div><code class="text-blue-600">t1</code>, <code class="text-blue-600">t2</code> ... : 数据源别名 (Source aliases defined in rule).</div>
        </a-alert>

        <div class="function-category mb-8">
          <div class="category-header mb-4 pb-2 border-b-2 border-gray-200">
            <div class="text-xl font-semibold text-gray-800">1. 位操作函数 (Bitwise Operations)</div>
          </div>
          <a-table 
            :columns="docsColumns" 
            :data="bitwiseFunctions" 
            size="small" 
            :bordered="false" 
            class="function-table"
          >
            <template #function="{ record }">
              <span v-html="record.function"></span>
            </template>
            <template #description="{ record }">
              <span v-html="record.description"></span>
            </template>
            <template #example="{ record }">
              <div class="example-cell">
                <span v-html="record.example"></span>
                <a-button 
                  type="text" 
                  size="small" 
                  class="copy-button" 
                  @click="copyExample(record.example)"
                  title="复制示例"
                >
                  复制
                </a-button>
              </div>
            </template>
          </a-table>
        </div>

        <div class="function-category mb-8">
          <div class="category-header mb-4 pb-2 border-b-2 border-gray-200">
            <div class="text-xl font-semibold text-gray-800">2. 数学函数 (Mathematical Functions)</div>
          </div>
          <a-table 
            :columns="docsColumns" 
            :data="mathFunctions" 
            size="small" 
            :bordered="false" 
            class="function-table"
          >
            <template #function="{ record }">
              <span v-html="record.function"></span>
            </template>
            <template #description="{ record }">
              <span v-html="record.description"></span>
            </template>
            <template #example="{ record }">
              <div class="example-cell">
                <span v-html="record.example"></span>
                <a-button 
                  type="text" 
                  size="small" 
                  class="copy-button" 
                  @click="copyExample(record.example)"
                  title="复制示例"
                >
                  复制
                </a-button>
              </div>
            </template>
          </a-table>
        </div>

        <div class="function-category mb-8">
          <div class="category-header mb-4 pb-2 border-b-2 border-gray-200">
            <div class="text-xl font-semibold text-gray-800">3. 逻辑函数 (Logical Functions)</div>
          </div>
          <a-table 
            :columns="docsColumns" 
            :data="logicalFunctions" 
            size="small" 
            :bordered="false" 
            class="function-table"
          >
            <template #function="{ record }">
              <span v-html="record.function"></span>
            </template>
            <template #description="{ record }">
              <span v-html="record.description"></span>
            </template>
            <template #example="{ record }">
              <div class="example-cell">
                <span v-html="record.example"></span>
                <a-button 
                  type="text" 
                  size="small" 
                  class="copy-button" 
                  @click="copyExample(record.example)"
                  title="复制示例"
                >
                  复制
                </a-button>
              </div>
            </template>
          </a-table>
        </div>
      </div>
      <template #footer>
        <a-button type="primary" @click="docsDialog = false" class="w-24">关闭</a-button>
      </template>
    </a-modal>

    <a-modal
      v-model:visible="deleteDialog.visible"
      title="确认删除"
      ok-text="确认删除"
      cancel-text="取消"
      :ok-button-props="{ status: 'danger', loading: deleteDialog.loading }"
      @ok="executeDeleteRule"
      @cancel="deleteDialog.visible = false"
    >
      <template v-if="deleteDialog.isBatch">
        确定要批量删除选中的 <span class="text-red-500 font-bold">{{ deleteDialog.batchCount }}</span> 条规则吗？
      </template>
      <template v-else>
        确定要删除规则 <span class="text-red-500 font-bold">{{ deleteDialog.rule?.name || deleteDialog.rule?.id }}</span> 吗？
      </template>
      <div class="mt-2 text-gray-400 text-sm">此操作不可撤销。</div>
    </a-modal>
  </div>
</template>

<script setup>
import {
  Tabs, TabPane, Card, Table, Button, Modal, Form, FormItem, Input,
  InputNumber, Select, Switch, Alert, Collapse, CollapseItem, Tag, Row, Col,
  RadioGroup, Radio, Empty
} from '@arco-design/web-vue'
import {
  IconPlus, IconEdit, IconDelete, IconRefresh, IconDownload, IconCode, IconInfoCircle, IconBook, IconArrowDown, IconQuestionCircle, IconFilter,
  IconStorage, IconHistory, IconClockCircle, IconUpload, IconFile
} from '@arco-design/web-vue/es/icon'
import ActionEditor from '@/components/ActionEditor.vue'
import EdgeComputeHelpDrawer from '@/components/edge-compute/EdgeComputeHelpDrawer.vue'
import EdgeComputePageFooter from '@/components/edge-compute/EdgeComputePageFooter.vue'
import EdgeSceneTemplates from '@/components/edge-compute/EdgeSceneTemplates.vue'
import { cloneSceneTemplateRule } from '@/utils/edgeSceneTemplates'
import EdgeRecordFilterModal from '@/components/edge-compute/EdgeRecordFilterModal.vue'
import RuleFlowCard from '@/components/edge-compute/RuleFlowCard.vue'
import {
  buildEventApiParams,
  buildLogApiParams,
  countActiveFilters,
  createDefaultFilters,
  describeFilterChips,
  filterEventsClient,
  filterLogsClient,
  LOG_ERROR_TYPE_LABELS,
} from '@/composables/useEdgeRecordFilters'

// 表格列定义
const ruleColumns = [
  { title: '规则名称', dataIndex: 'name' },
  { title: '类型', dataIndex: 'type', slotName: 'type' },
  { title: '触发模式', dataIndex: 'trigger_mode', slotName: 'trigger_mode' },
  { title: '启用状态', dataIndex: 'enable', slotName: 'enable' },
  { title: '优先级', dataIndex: 'priority' },
  { title: '操作', dataIndex: 'operations', slotName: 'operations', fixed: 'right', width: 120 }
]

const logColumns = [
  { title: '时间', dataIndex: 'minute', width: 160 },
  { title: '规则ID', dataIndex: 'rule_id', width: 120 },
  { title: '规则名称', dataIndex: 'rule_name', width: 160 },
  { title: '错误类型', dataIndex: 'error_type', slotName: 'error_type', width: 140 },
  { title: '错误信息', dataIndex: 'error_message', slotName: 'error_message', width: 480 }
]

const eventColumns = [
  { title: '时间', dataIndex: 'started_at', slotName: 'started_at', width: 160 },
  { title: '规则', dataIndex: 'rule_name', ellipsis: true },
  { title: '状态', dataIndex: 'status', slotName: 'status', width: 90 },
  { title: '耗时', dataIndex: 'duration_ms', slotName: 'duration_ms', width: 80 },
  { title: '动作', dataIndex: 'actions', slotName: 'actions', ellipsis: true },
  { title: '错误', dataIndex: 'error_message', slotName: 'error_message', ellipsis: true },
]

const windowDataColumns = [
  { title: '时间', dataIndex: 'ts', slotName: 'ts' },
  { title: '值', dataIndex: 'value' }
]

const docsColumns = [
  { 
    title: '函数 (Function)', 
    dataIndex: 'function', 
    width: 200,
    slotName: 'function'
  },
  { 
    title: '说明 (Description)', 
    dataIndex: 'description',
    slotName: 'description'
  },
  { 
    title: '示例 (Example)', 
    dataIndex: 'example',
    slotName: 'example'
  }
]

const bitwiseFunctions = [
  { function: '<code>bitand(a, b)</code>', description: '按位与 (Bitwise AND). 对应 <code>a & b</code>', example: '<code>bitand(v, 1)</code> (判断最低位是否为1)' },
  { function: '<code>bitor(a, b)</code>', description: '按位或 (Bitwise OR). 对应 <code>a | b</code>', example: '<code>bitor(v, 4)</code> (将第3位置1)' },
  { function: '<code>bitxor(a, b)</code>', description: '按位异或 (Bitwise XOR). 对应 <code>a ^ b</code>', example: '<code>bitxor(v, 1)</code> (翻转最低位)' },
  { function: '<code>bitnot(a)</code>', description: '按位非 (Bitwise NOT). 对应 <code>~a</code>', example: '<code>bitnot(v)</code> (翻转所有位)' },
  { function: '<code>bitget(v, n)</code>', description: '获取第 n 位 (0-based)', example: '<code>bitget(v, 3)</code> (获取第4位)' },
  { function: '<code>bitset(v, n)</code>', description: '将第 n 位置 1', example: '<code>bitset(v, 3)</code> (将第4位置1)' },
  { function: '<code>bitclr(v, n)</code>', description: '将第 n 位置 0', example: '<code>bitclr(v, 3)</code> (将第4位置0)' }
]

const mathFunctions = [
  { function: '<code>abs(a)</code>', description: '绝对值', example: '<code>abs(v)</code>' },
  { function: '<code>ceil(a)</code>', description: '向上取整', example: '<code>ceil(v)</code>' },
  { function: '<code>floor(a)</code>', description: '向下取整', example: '<code>floor(v)</code>' },
  { function: '<code>round(a)</code>', description: '四舍五入', example: '<code>round(v)</code>' },
  { function: '<code>sqrt(a)</code>', description: '平方根', example: '<code>sqrt(v)</code>' },
  { function: '<code>pow(a, b)</code>', description: '幂运算', example: '<code>pow(v, 2)</code> (平方)' },
  { function: '<code>sin(a)</code>', description: '正弦函数', example: '<code>sin(v)</code>' },
  { function: '<code>cos(a)</code>', description: '余弦函数', example: '<code>cos(v)</code>' },
  { function: '<code>tan(a)</code>', description: '正切函数', example: '<code>tan(v)</code>' }
]

const logicalFunctions = [
  { function: '<code>and(a, b)</code>', description: '逻辑与', example: '<code>and(t1 > 50, t2 > 30)</code>' },
  { function: '<code>or(a, b)</code>', description: '逻辑或', example: '<code>or(t1 > 50, t2 > 30)</code>' },
  { function: '<code>not(a)</code>', description: '逻辑非', example: '<code>not(t1 > 50)</code>' },
  { function: '<code>eq(a, b)</code>', description: '等于', example: '<code>eq(v, 1)</code>' },
  { function: '<code>ne(a, b)</code>', description: '不等于', example: '<code>ne(v, 0)</code>' },
  { function: '<code>gt(a, b)</code>', description: '大于', example: '<code>gt(v, 50)</code>' },
  { function: '<code>ge(a, b)</code>', description: '大于等于', example: '<code>ge(v, 50)</code>' },
  { function: '<code>lt(a, b)</code>', description: '小于', example: '<code>lt(v, 50)</code>' },
  { function: '<code>le(a, b)</code>', description: '小于等于', example: '<code>le(v, 50)</code>' }
]
import { ref, reactive, computed, watch, onMounted, provide } from 'vue'
import { useRoute } from 'vue-router'
import request from '@/utils/request'
import { showMessage } from '@/composables/useGlobalState'
import { useEdgeStatePolling } from '@/composables/useEdgeStatePolling'
import { base64ToUint8Array, uint8ArrayToHex, detectFileType, downloadBytes } from '@/utils/decode'
import EdgeComputeMetrics from './EdgeComputeMetrics.vue'

// Expression Helper
const helperDialog = ref(false)
const docsDialog = ref(false)
const helperInput = ref('')
const helperOutput = ref('')
let helperCallback = null

const openHelper = (initialValue, callback) => {
    helperInput.value = initialValue || ''
    helperOutput.value = ''
    helperCallback = callback
    helperDialog.value = true
}

const convertHelper = () => {
    let res = helperInput.value
    if (!res) return

    // Handle ~ (Unary NOT)
    res = res.replace(/~\s*([a-zA-Z0-9_.]+|\([^)]+\))/g, 'bitnot($1)')

    let prev = ''
    let limit = 10
    while (prev !== res && limit > 0) {
        prev = res
        limit--
        
        // << and >>
        res = res.replace(/([a-zA-Z0-9_.]+|\([^)]+\))\s*(<<|>>)\s*([a-zA-Z0-9_.]+|\([^)]+\))/g, (m, a, op, b) => {
             return op === '<<' ? `bitshl(${a}, ${b})` : `bitshr(${a}, ${b})`
        })
        
        // &
        res = res.replace(/([a-zA-Z0-9_.]+|\([^)]+\))\s*&\s*([a-zA-Z0-9_.]+|\([^)]+\))/g, 'bitand($1, $2)')
        
        // ^
        res = res.replace(/([a-zA-Z0-9_.]+|\([^)]+\))\s*\^\s*([a-zA-Z0-9_.]+|\([^)]+\))/g, 'bitxor($1, $2)')
        
        // |
        res = res.replace(/([a-zA-Z0-9_.]+|\([^)]+\))\s*\|\s*([a-zA-Z0-9_.]+|\([^)]+\))/g, 'bitor($1, $2)')
    }
    helperOutput.value = res
}

const applyHelper = () => {
    if (helperCallback && helperOutput.value) {
        helperCallback(helperOutput.value)
    }
    helperDialog.value = false
}

const copyExample = (example) => {
    // 提取代码内容，去除 HTML 标签
    const tempElement = document.createElement('div')
    tempElement.innerHTML = example
    const text = tempElement.textContent || tempElement.innerText
    
    // 复制到剪贴板
    navigator.clipboard.writeText(text).then(() => {
        showMessage('示例已复制到剪贴板', 'success')
    }).catch(err => {
        console.error('复制失败:', err)
        showMessage('复制失败，请手动复制', 'error')
    })
}

const route = useRoute()
const tab = ref('metrics')
const rulesViewMode = ref('flow')
const rules = ref([])
const ruleStates = ref([])

const ruleStateMap = computed(() => {
    const map = {}
    for (const state of ruleStates.value) {
        if (state?.rule_id) map[state.rule_id] = state
    }
    return map
})

const toggleRuleSelection = (ruleId, checked) => {
    const idx = selectedRuleKeys.value.indexOf(ruleId)
    if (checked && idx === -1) {
        selectedRuleKeys.value = [...selectedRuleKeys.value, ruleId]
    } else if (!checked && idx !== -1) {
        selectedRuleKeys.value = selectedRuleKeys.value.filter(id => id !== ruleId)
    }
}
const dialog = ref(false)
const helpVisible = ref(false)
const editingRule = ref(false)

// JSON Import State
const jsonImportVisible = ref(false)
const jsonImportMode = ref('paste')
const jsonImportContent = ref('')
const jsonImportPreview = ref([])
const jsonImportError = ref('')
const jsonFileName = ref('')

// Northbound Config for Actions
const northboundConfig = ref({ mqtt: [], http: [] })

// MQTT Options for Select
const mqttOptions = computed(() => {
    const cfg = northboundConfig.value || {}
    const list = Array.isArray(cfg.mqtt) ? cfg.mqtt : []
    const status = cfg.status || {}

    const options = list
        .filter(item => item.enable)   // ✅ 只要启用的
        .map(item => ({
            label: `${item.name || item.id}${status[item.id] === 3 ? '' : ' (离线)'}`,
            value: item.id,
            disabled: status[item.id] !== 3   // ✅ 工业级：离线不可选
        }))

    // ✅ 强制兜底（关键：保证一定有 Default MQTT）
    if (!options.find(o => o.label.includes('Default MQTT'))) {
        options.unshift({
            label: 'Default MQTT',
            value: 'mqtt-1'
        })
    }

    return options
})

// HTTP Options for Select
const httpOptions = computed(() => {
    const cfg = northboundConfig.value || {}
    const list = Array.isArray(cfg.http) ? cfg.http : []
    const status = cfg.status || {}

    const options = list
        .filter(item => item.enable)   // ✅ 只要启用的
        .map(item => ({
            label: `${item.name || item.id}${status[item.id] === 3 ? '' : ' (离线)'}`,
            value: item.id,
            disabled: status[item.id] !== 3   // ✅ 工业级：离线不可选
        }))

    return options
})

// Provide only UI data
provide('mqttOptions', mqttOptions)
provide('httpOptions', httpOptions)

const selectedRuleKeys = ref([]) // 批量选择存储

const channels = ref([])
const devices = ref([])
const windowDialog = ref(false)
const windowData = ref([])
const currentWindowRuleName = ref('')

const fetchNorthboundConfig = async () => {
    try {
        const data = await request.get('/api/northbound/config')
        if (data) {
            northboundConfig.value = {
                mqtt: data.mqtt || [],
                http: data.http || [],
                status: data.status || {}
            }
        }
    } catch (e) {
        console.error("Failed to fetch northbound config", e)
    }
}

// Details Dialog
const detailsDialog = ref(false)
const detailTitle = ref('')
const detailContent = ref('')
const detailsTab = ref('text')
const decodedHex = ref('')
const detectedFile = ref(null)
const decodedBytes = ref(null)

const showDetails = (title, content) => {
    detailTitle.value = title
    detailContent.value = String(content || '')
    detailsDialog.value = true
    
    // Reset state
    detailsTab.value = 'text'
    decodedHex.value = ''
    detectedFile.value = null
    decodedBytes.value = null
}

const tryDecode = () => {
    try {
        const bytes = base64ToUint8Array(detailContent.value)
        decodedBytes.value = bytes
        decodedHex.value = uint8ArrayToHex(bytes)
        detectedFile.value = detectFileType(bytes)
        
        detailsTab.value = 'hex'
        showMessage('解码成功', 'success')
    } catch (e) {
        showMessage('Base64 解码失败: ' + e.message, 'error')
    }
}

const downloadDetectedFile = () => {
    if (decodedBytes.value && detectedFile.value) {
        downloadBytes(decodedBytes.value, `download.${detectedFile.value.ext}`)
    }
}

const recordType = ref('events')
const filterVisible = ref(false)
const eventFilters = reactive(createDefaultFilters('events'))
const logFilters = reactive(createDefaultFilters('logs'))
const rawEdgeEvents = ref([])
const rawLogs = ref([])
const edgeEvents = computed(() => filterEventsClient(rawEdgeEvents.value, eventFilters))
const logs = computed(() => filterLogsClient(rawLogs.value, logFilters))
const currentRecordFilters = computed(() => (recordType.value === 'events' ? eventFilters : logFilters))
const activeFilterCount = computed(() => countActiveFilters(currentRecordFilters.value, recordType.value))
const activeFilterChips = computed(() => describeFilterChips(currentRecordFilters.value, recordType.value, rules.value))
const recordCount = computed(() => (recordType.value === 'events' ? edgeEvents.value.length : logs.value.length))

const currentRule = reactive({
    id: '',
    name: '',
    type: 'threshold',
    priority: 0,
    enable: true,
    trigger_mode: 'always',
    check_interval: '',
    sources: [], // Multi-source
    trigger_logic: 'EXPR', // Default to EXPR for custom logic
    condition: '',
    expression: '',
    window: { type: 'sliding', size: '10s', aggr_func: 'avg' },
    state: { duration: '0s', count: 0 },
    actions: []
})

const getStatusColor = (status) => {
    switch (status) {
        case 'ALARM': return 'error'
        case 'WARNING': return 'warning'
        case 'NORMAL': return 'success'
        default: return 'grey'
    }
}

const formatRuleType = (type) => {
    const map = {
        'threshold': 'Threshold (阈值触发)',
        'calculation': 'Calculation (计算公式)',
        'window': 'Window (时间/计数窗口)',
        'state': 'State (状态持续)'
    }
    return map[type] || type
}

const formatTriggerMode = (mode) => {
    const map = {
        'always': 'Always (始终触发)',
        'on_change': 'On Change (仅状态改变时触发)'
    }
    return map[mode] || mode
}

const getRuleTypeExplanation = (type) => {
    const map = {
        'threshold': '当数值满足条件表达式时触发。适用于简单的越限报警。',
        'calculation': '计算新值并输出，始终触发。适用于数据预处理或单位转换。',
        'window': '在指定时间或次数窗口内聚合数据（如求平均值）',
        'state': '当条件持续满足指定时间后触发。适用于防抖动报警。'
    }
    return map[type] || ''
}

const fetchChannels = async () => {
    try {
        const data = await request.get('/api/channels')
        if (data) {
            channels.value = (data || []).map(ch => ({
                label: ch.name || ch.channel_name || ch.id,
                value: ch.id,
                raw: ch
            }))
        }
    } catch (e) {
        console.error(e)
    }
}

// Source Management
const addSource = () => {
    if (!currentRule.sources) currentRule.sources = []
    currentRule.sources.push({
        channel_id: '',
        device_id: '',
        point_id: '',
        alias: '',
        _deviceList: [], // Local state for dropdown
        _pointList: []
    })
}

const removeSource = (index) => {
    currentRule.sources.splice(index, 1)
}

const detectInvalidSources = () => {
    if (!currentRule.sources) return
    
    let invalidCount = 0
    currentRule.sources.forEach((src, index) => {
        const isInvalid = !src.channel_id || !src.device_id || !src.point_id || !src.alias
        if (isInvalid) {
            invalidCount++
        }
    })
    
    if (invalidCount > 0) {
        showMessage(`检测到 ${invalidCount} 个失效配置，请点击"一键清除"按钮清理`, 'warning')
    } else {
        showMessage('所有数据源配置均有效', 'success')
    }
}

const clearInvalidSources = () => {
    if (!currentRule.sources) return
    
    const validSources = currentRule.sources.filter(src => 
        src.channel_id && src.device_id && src.point_id && src.alias
    )
    
    const removedCount = currentRule.sources.length - validSources.length
    currentRule.sources = validSources
    
    if (removedCount > 0) {
        showMessage(`已清除 ${removedCount} 个失效配置`, 'success')
    } else {
        showMessage('没有发现失效配置', 'info')
    }
}

const onSourceChannelChange = async (src) => {
    src.device_id = ''
    src.point_id = ''
    src._deviceList = []
    src._pointList = []
    
    if (!src.channel_id) return
    
    try {
        const data = await request.get(`/api/channels/${src.channel_id}/devices`)
        if (data) {
            src._deviceList = (Array.isArray(data) ? data : []).map(d => ({
                label: d.name || d.device_name || d.id,
                value: d.id,
                raw: d
            }))
        }
    } catch (e) {
        console.error(e)
    }
}

const onSourceDeviceChange = (src) => {
    src.point_id = ''
    src._pointList = []
    updateSourcePointList(src)
}

const updateSourcePointList = (src) => {
    if (!src.device_id || !src._deviceList || !Array.isArray(src._deviceList)) return
    const dev = src._deviceList.find(d => d.value === src.device_id)
    if (dev && dev.raw && dev.raw.points) {
        src._pointList = (dev.raw.points || []).map(p => ({
            label: p.name || p.point_name || p.id,
            value: p.id,
            raw: p
        }))
    } else {
        src._pointList = []
    }
}

const loadSourceDevices = async (src) => {
    if (!src.channel_id || (src._deviceList && src._deviceList.length > 0)) return
    await onSourceChannelChange(src)
}

const loadSourcePoints = (src) => {
    // No-op or alias for updateSourcePointList, but strictly we don't need to load from network
    // providing we have device list.
    if (!src._pointList || src._pointList.length === 0) {
        updateSourcePointList(src)
    }
}

const fetchRules = async () => {
    try {
        const data = await request.get('/api/edge/rules')
        if (data) {
            rules.value = data
        }
    } catch (e) {
        console.error(e)
    }
}

const fetchRuleStates = async () => {
    try {
        const data = await request.get('/api/edge/states')
        if (data) {
            ruleStates.value = Object.values(data)
        }
    } catch (e) {
        console.error(e)
    }
}

const fetchEdgeEvents = async () => {
    try {
        const params = buildEventApiParams(eventFilters)
        const data = await request.get(`/api/edge/events?${params.toString()}`)
        rawEdgeEvents.value = data || []
    } catch (e) {
        console.error('Failed to fetch edge events', e)
    }
}

const refreshRecords = async () => {
    if (recordType.value === 'events') {
        await fetchEdgeEvents()
    } else {
        await queryLogs()
    }
}

const refreshRuntimeRecords = async () => {
    await Promise.all([fetchRuleStates(), refreshRecords()])
}

const formatErrorType = (errorType) => LOG_ERROR_TYPE_LABELS[errorType] || errorType || '其他错误'

const getEventStatusColor = (status) => {
    switch (status) {
        case 'error': return 'red'
        case 'dropped': return 'orange'
        default: return 'gray'
    }
}

const formatEventActions = (actions) => {
    if (!actions?.length) return '—'
    return actions.map(a => `${a.type}:${a.status}`).join(', ')
}

const { refresh: refreshRuleStates } = useEdgeStatePolling({
    tab,
    rulesViewMode,
    fetchRuleStates: refreshRuntimeRecords,
})

const queryLogs = async () => {
    try {
        const params = buildLogApiParams(logFilters)
        const data = await request.get(`/api/edge/logs?${params.toString()}`)
        rawLogs.value = data || []
    } catch (e) {
        console.error('Failed to query logs', e)
    }
}

const applyRecordFilters = async (filters) => {
    Object.assign(currentRecordFilters.value, filters)
    await refreshRecords()
}

const removeFilterChip = async (key) => {
    const filters = currentRecordFilters.value
    if (key === 'limit') {
        filters.limit = 100
    } else if (key === 'categories') {
        filters.categories = []
    } else {
        filters[key] = ''
    }
    await refreshRecords()
}

const resetRecordFilters = async () => {
    Object.assign(currentRecordFilters.value, createDefaultFilters(recordType.value))
    await refreshRecords()
}

const clearEdgeLogs = async () => {
    try {
        const res = await request.post('/api/edge/logs/clear')
        rawEdgeEvents.value = []
        rawLogs.value = []
        let msg = '日志已清空'
        if (res.compact?.saved_size && res.compact.saved_size !== '0 MB') {
            msg += `，runtime.db 已压缩（节省 ${res.compact.saved_size}）`
        } else if (res.compact) {
            msg += '，runtime.db 已压缩'
        }
        showMessage(msg, 'success')
    } catch (e) {
        showMessage('清空失败: ' + (e.message || e), 'error')
    }
}

const exportLogs = async () => {
    if (!logs.value || logs.value.length === 0) return

    const headers = ['Time', 'Rule ID', 'Rule Name', 'Error Type', 'Error Message']
    const csvContent = [
        headers.join(','),
        ...logs.value.map(log => [
            log.minute,
            log.rule_id,
            log.rule_name,
            log.error_type,
            `"${(log.error_message || '').replace(/"/g, '""')}"`
        ].join(','))
    ].join('\n')

    const blob = new Blob([csvContent], { type: 'text/csv;charset=utf-8;' })
    const link = document.createElement('a')
    if (link.download !== undefined) {
        const url = URL.createObjectURL(blob)
        link.setAttribute('href', url)
        link.setAttribute('download', `edge_logs_${new Date().toISOString().slice(0,19).replace(/[:T]/g, '-')}.csv`)
        link.style.visibility = 'hidden'
        document.body.appendChild(link)
        link.click()
        document.body.removeChild(link)
    }
}

const viewWindowData = async (ruleId, ruleName) => {
    currentWindowRuleName.value = ruleName
    windowData.value = []
    windowDialog.value = true
    try {
        const data = await request.get(`/api/edge/rules/${ruleId}/window`)
        if (data) {
            windowData.value = data
        }
    } catch (e) {
        console.error(e)
    }
}

const addAction = () => {
    if (!currentRule.actions) currentRule.actions = []
    currentRule.actions.push({
        type: 'log',
        config: {}
    })
}

const removeAction = (index) => {
    currentRule.actions.splice(index, 1)
}

const detectInvalidActions = () => {
    if (!currentRule.actions) return
    
    let invalidCount = 0
    currentRule.actions.forEach(action => {
        const isInvalid = isActionInvalid(action)
        if (isInvalid) {
            invalidCount++
        }
    })
    
    if (invalidCount > 0) {
        showMessage(`检测到 ${invalidCount} 个失效动作配置，请点击"一键清除"按钮清理`, 'warning')
    } else {
        showMessage('所有动作配置均有效', 'success')
    }
}

const clearInvalidActions = () => {
    if (!currentRule.actions) return
    
    const validActions = currentRule.actions.filter(action => !isActionInvalid(action))
    const removedCount = currentRule.actions.length - validActions.length
    currentRule.actions = validActions
    
    if (removedCount > 0) {
        showMessage(`已清除 ${removedCount} 个失效动作配置`, 'success')
    } else {
        showMessage('没有发现失效动作配置', 'info')
    }
}

const isActionInvalid = (action) => {
    if (!action.type) return true
    
    switch (action.type) {
        case 'mqtt':
            return !action.config.mqtt_id
        case 'http':
            return !action.config.http_id
        case 'device_control':
            return !action.config.channel_id || !action.config.device_id || !action.config.point_id
        case 'check':
            return !action.config.condition
        case 'delay':
            return !action.config.duration
        case 'sequence':
            return !action.config.steps || !Array.isArray(action.config.steps) || action.config.steps.length === 0
        default:
            return false
    }
}

const resetCurrentRule = () => {
    currentRule.id = ''
    currentRule.name = ''
    currentRule.type = 'threshold'
    currentRule.priority = 0
    currentRule.enable = true
    currentRule.trigger_mode = 'always'
    currentRule.check_interval = ''
    currentRule.sources = []
    currentRule.trigger_logic = 'EXPR'
    currentRule.condition = ''
    currentRule.expression = ''
    currentRule.window = { type: 'sliding', size: '10s', aggr_func: 'avg' }
    currentRule.state = { duration: '0s', count: 0 }
    currentRule.actions = []
}

const openDialog = () => {
    editingRule.value = false
    resetCurrentRule()
    addSource()
    dialog.value = true
}

const applyRuleTemplate = (template) => {
    editingRule.value = false
    resetCurrentRule()
    Object.assign(currentRule, cloneSceneTemplateRule(template))
    tab.value = 'rules'
    dialog.value = true
    showMessage(`已加载模版「${template.name}」，请绑定数据源与动作目标后保存`, 'info')
}

const editRule = async (rule) => {
    editingRule.value = true
    // Deep copy
    const r = JSON.parse(JSON.stringify(rule))
    Object.assign(currentRule, r)
    
    // Ensure nested objects
    if (!currentRule.sources) currentRule.sources = []
    // Backward compatibility: If source exists but sources is empty
    if (currentRule.sources.length === 0 && r.source && r.source.channel_id) {
        currentRule.sources.push({
            channel_id: r.source.channel_id,
            device_id: r.source.device_id,
            point_id: r.source.point_id,
            alias: r.source.alias || 'val'
        })
    }
    
    if (!currentRule.actions) currentRule.actions = []

    if (!currentRule.window) currentRule.window = { type: 'sliding', size: '10s', aggr_func: 'avg' }
    if (!currentRule.state) currentRule.state = { duration: '0s', count: 0 }
    
    // Load metadata for sources (devices/points list)
    for (const src of currentRule.sources) {
        if (src.channel_id) {
            src._deviceList = await fetchDevices(src.channel_id)
            if (src.device_id) {
                updateSourcePointList(src)
            }
        }
    }
    
    dialog.value = true
}

const deleteDialog = reactive({
    visible: false,
    isBatch: false,
    rule: null,
    batchCount: 0,
    loading: false
})

const deleteRule = (rule) => {
    deleteDialog.isBatch = false
    deleteDialog.rule = rule
    deleteDialog.visible = true
}

const handleBatchDelete = () => {
    if (selectedRuleKeys.value.length === 0) return
    deleteDialog.isBatch = true
    deleteDialog.batchCount = selectedRuleKeys.value.length
    deleteDialog.visible = true
}

const executeDeleteRule = async () => {
    if (deleteDialog.loading) return
    deleteDialog.loading = true
    try {
        if (deleteDialog.isBatch) {
            await Promise.all(selectedRuleKeys.value.map(id => request.delete(`/api/edge/rules/${id}`)))
            selectedRuleKeys.value = []
            showMessage('批量删除成功', 'success')
        } else if (deleteDialog.rule) {
            await request.delete(`/api/edge/rules/${deleteDialog.rule.id}`)
            showMessage(`规则「${deleteDialog.rule.name || deleteDialog.rule.id}」已删除`, 'success')
        }
        deleteDialog.visible = false
        fetchRules()
    } catch (e) {
        showMessage('删除失败: ' + (e.message || e), 'error')
    } finally {
        deleteDialog.loading = false
    }
}

const ruleSaving = ref(false)

const saveRule = async () => {
    if (ruleSaving.value) return
    ruleSaving.value = true
    try {
        await request.post('/api/edge/rules', currentRule)
        dialog.value = false
        fetchRules()
        showMessage('规则保存成功', 'success')
    } catch (e) {
        showMessage('保存失败: ' + e.message, 'error')
    } finally {
        ruleSaving.value = false
    }
}

// ── JSON Import ──

const getRuleTypeTagColor = (type) => {
    const map = { threshold: 'red', calculation: 'arcoblue', window: 'orange', state: 'green' }
    return map[type] || 'gray'
}

// 校验单条规则 JSON 结构
const validateJsonRule = (rule, index) => {
    if (!rule || typeof rule !== 'object') {
        return `第 ${index + 1} 条规则不是有效的 JSON 对象`
    }
    if (!rule.name || !rule.name.trim()) {
        return `第 ${index + 1} 条规则缺少 name 字段`
    }
    const validTypes = ['threshold', 'calculation', 'window', 'state']
    if (rule.type && !validTypes.includes(rule.type)) {
        return `第 ${index + 1} 条规则 type 无效: "${rule.type}"，有效值为: ${validTypes.join(', ')}`
    }
    return null
}

// 解析并校验 JSON 内容
const parseJsonRules = (raw) => {
    if (!raw || !raw.trim()) {
        return { error: '请输入 JSON 内容', rules: [] }
    }
    let parsed
    try {
        parsed = JSON.parse(raw)
    } catch (e) {
        return { error: `JSON 解析失败: ${e.message}`, rules: [] }
    }

    let rules = []
    if (Array.isArray(parsed)) {
        rules = parsed
    } else if (typeof parsed === 'object' && parsed !== null) {
        rules = [parsed]
    } else {
        return { error: 'JSON 格式不支持，请提供规则对象或规则数组', rules: [] }
    }

    if (rules.length === 0) {
        return { error: '未找到任何规则数据', rules: [] }
    }

    // 校验每条规则
    for (let i = 0; i < rules.length; i++) {
        const err = validateJsonRule(rules[i], i)
        if (err) {
            return { error: err, rules: [] }
        }
    }

    return { error: null, rules }
}

// 文本粘贴模式：内容变化时自动解析预览
const onJsonInputChange = () => {
    const { error, rules } = parseJsonRules(jsonImportContent.value)
    jsonImportError.value = error || ''
    jsonImportPreview.value = error ? [] : rules
}

// 打开 JSON 导入弹窗
const openJsonImport = () => {
    jsonImportVisible.value = true
    jsonImportMode.value = 'paste'
    jsonImportContent.value = ''
    jsonImportPreview.value = []
    jsonImportError.value = ''
    jsonFileName.value = ''
}

// 清除 JSON 导入内容
const clearJsonImport = () => {
    jsonImportContent.value = ''
    jsonImportPreview.value = []
    jsonImportError.value = ''
    jsonFileName.value = ''
}

// 文件拖拽处理
const handleJsonFileDrop = (e) => {
    const files = e.dataTransfer?.files
    if (files && files.length > 0) {
        readJsonFile(files[0])
    }
}

// 文件选择处理 (Arco Upload @change 回调签名: (fileItemList, fileItem))
const handleJsonFileSelect = (fileItemList, fileItem) => {
    const item = fileItem || (Array.isArray(fileItemList) ? fileItemList[0] : null)
    const file = item?.file || item?.originFile
    if (file) {
        readJsonFile(file)
    }
}

// 读取 JSON 文件
const readJsonFile = (file) => {
    if (!file.name.endsWith('.json')) {
        jsonImportError.value = '请选择 .json 文件'
        return
    }
    jsonFileName.value = file.name
    jsonImportError.value = ''

    const reader = new FileReader()
    reader.onload = (e) => {
        jsonImportContent.value = e.target.result
        const { error, rules } = parseJsonRules(jsonImportContent.value)
        jsonImportError.value = error || ''
        jsonImportPreview.value = error ? [] : rules
    }
    reader.onerror = () => {
        jsonImportError.value = '文件读取失败'
    }
    reader.readAsText(file)
}

// 清除已选文件
const clearJsonFile = () => {
    jsonFileName.value = ''
    jsonImportContent.value = ''
    jsonImportPreview.value = []
    jsonImportError.value = ''
}

// 执行批量导入
const jsonImporting = ref(false)

const executeJsonImport = async () => {
    if (jsonImporting.value) return
    if (jsonImportPreview.value.length === 0) {
        showMessage('没有可导入的规则', 'warning')
        return
    }
    jsonImporting.value = true

    let successCount = 0
    let failCount = 0

    for (const rule of jsonImportPreview.value) {
        try {
            // 填充默认值，确保规则结构完整
            const payload = {
                name: rule.name,
                type: rule.type || 'threshold',
                enable: rule.enable !== false,
                priority: rule.priority ?? 0,
                trigger_mode: rule.trigger_mode || 'always',
                check_interval: rule.check_interval || '',
                sources: rule.sources || [],
                trigger_logic: rule.trigger_logic || 'EXPR',
                condition: rule.condition || '',
                expression: rule.expression || '',
                window: rule.window || { type: 'sliding', size: '10s', aggr_func: 'avg' },
                state: rule.state || { duration: '0s', count: 0 },
                actions: rule.actions || []
            }
            await request.post('/api/edge/rules', payload)
            successCount++
        } catch (e) {
            failCount++
            console.error(`导入规则 "${rule.name}" 失败:`, e)
        }
    }

    if (failCount === 0) {
        showMessage(`成功导入 ${successCount} 条规则`, 'success')
    } else {
        showMessage(`导入完成: ${successCount} 条成功, ${failCount} 条失败`, 'warning')
    }

    jsonImporting.value = false
    jsonImportVisible.value = false
    fetchRules()
}

const handleBatchEnable = async (status) => {
    if (selectedRuleKeys.value.length === 0) return
    
    try {
        await Promise.all(selectedRuleKeys.value.map(id => request.put(`/api/edge/rules/${id}`, { enable: status })))
        selectedRuleKeys.value = []
        fetchRules()
        showMessage(`批量${status ? '启用' : '禁用'}成功`, 'success')
    } catch (e) {
        showMessage(`批量${status ? '启用' : '禁用'}失败: ` + e.message, 'error')
    }
}

const getLogicColor = (logic) => {
    switch(logic) {
        case 'AND': return 'info'
        case 'OR': return 'warning'
        case 'EXPR': return 'purple'
        default: return 'grey'
    }
}

const formatDate = (ts) => {
    if (!ts || ts === '0001-01-01T00:00:00Z') return '-'
    return new Date(ts).toLocaleString()
}

const calculateDuration = (startTs) => {
    if (!startTs || startTs === '0001-01-01T00:00:00Z') return '-'
    const start = new Date(startTs).getTime()
    const now = new Date().getTime()
    const diff = Math.floor((now - start) / 1000)
    
    if (diff < 60) return `${diff}s`
    if (diff < 3600) return `${Math.floor(diff/60)}m ${diff%60}s`
    return `${Math.floor(diff/3600)}h ${Math.floor((diff%3600)/60)}m`
}

// Helper to fetch devices
const fetchDevices = async (channelId) => {
    if (!channelId) return []
    try {
        const data = await request.get(`/api/channels/${channelId}/devices`)
        if (data) {
            return data
        }
    } catch (e) {
        console.error(e)
    }
    return []
}

// Action Helper Functions removed - moved to ActionEditor component

watch(recordType, () => {
    if (tab.value === 'status') {
        refreshRecords()
    }
})

watch(tab, (newTab) => {
    if (newTab === 'status') {
        refreshRecords()
    }
})

onMounted(async () => {
    await fetchRules()
    fetchRuleStates()
    fetchChannels()
    fetchNorthboundConfig()

    if (route.query.tab === 'logs') {
        tab.value = 'status'
        recordType.value = 'logs'
    } else if (route.query.tab === 'status') {
        tab.value = 'status'
    } else if (route.query.tab === 'templates') {
        tab.value = 'templates'
    } else if (route.query.tab === 'rules') {
        tab.value = 'rules'
    } else if (route.query.tab === 'metrics') {
        tab.value = 'metrics'
    }

    if (route.query.rule) {
        const rule = rules.value.find(r => r.id === route.query.rule)
        if (rule) {
            editRule(rule)
        }
    }
})
</script>

<style scoped>
/* v3.0 — styles in src/styles/ */
</style>

