<template>
  <div class="page-shell virtual-shadow-page">
    <div class="page-header">
      <div>
        <h2 class="title-text">虚拟影子设备</h2>
        <p class="subtitle">
          从其它设备选点拼积木：直接映射来源点位，或通过公式计算生成新的虚拟点位
        </p>
      </div>
      <div class="header-actions virtual-shadow-header-actions">
        <a-space size="small">
          <a-radio-group v-model="viewMode" type="button" size="small">
            <a-radio value="card">
              <svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2">
                <rect x="3" y="3" width="7" height="7" />
                <rect x="14" y="3" width="7" height="7" />
                <rect x="14" y="14" width="7" height="7" />
                <rect x="3" y="14" width="7" height="7" />
              </svg>
            </a-radio>
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
          </a-radio-group>
        </a-space>
        <a-space>
          <a-button type="text" size="small" class="help-trigger-btn" @click="helpVisible = true">
            <template #icon><icon-question-circle /></template>
            帮助说明
          </a-button>
          <a-button @click="refreshAllRuntimes" :loading="loading">
            <template #icon><icon-refresh /></template>
            刷新当前值
          </a-button>
          <a-button type="primary" @click="openBuilder()">
            <template #icon><icon-plus /></template>
            新建虚拟设备
          </a-button>
        </a-space>
      </div>
    </div>

    <a-alert type="info" class="mb-4" closable>
      <template #title>快速建立虚拟设备</template>
      ① 搜索并选择<strong>源设备</strong> → ② 勾选点位（可批量）→ ③ 拖入右侧<strong>批量映射区</strong>自动生成映射积木，或拖入单个积木精调。
    </a-alert>

    <a-spin :loading="loading" style="width: 100%">
      <div v-if="!loading && devices.length === 0" class="empty-card">
        <div class="empty-content">
          <icon-apps :size="48" style="margin-bottom: 12px;" />
          <p>暂无虚拟影子设备</p>
          <a-button type="primary" @click="openBuilder()">
            <template #icon><icon-plus /></template>
            新建虚拟设备
          </a-button>
        </div>
      </div>

      <div v-else-if="viewMode === 'card'" class="vs-devices-grid">
        <div v-for="record in devices" :key="record.id" class="vs-device-card">
          <div class="vs-device-card-header">
            <div class="vs-device-icon">
              <icon-apps :size="20" />
            </div>
            <div class="vs-device-info">
              <div class="vs-device-name">
                <a-tooltip :content="nameTooltip(record)">
                  <span class="cell-ellipsis">{{ displayName(record) }}</span>
                </a-tooltip>
                <a-tag :color="record.enable ? 'green' : 'gray'" size="small" bordered>
                  {{ record.enable ? '启用' : '禁用' }}
                </a-tag>
              </div>
              <div v-if="record.description" class="vs-device-meta">
                <a-tooltip :content="record.description">
                  <span class="cell-ellipsis">{{ record.description }}</span>
                </a-tooltip>
              </div>
            </div>
          </div>

          <div class="vs-device-stats">
            <div class="stat-item">
              <div class="stat-item-label">点位</div>
              <div class="stat-item-value">{{ record.points?.length || 0 }}</div>
            </div>
            <div class="stat-item">
              <div class="stat-item-label">运行时</div>
              <div class="stat-item-value">
                <span v-if="runtimeMap[record.id]" class="runtime-badge">
                  v{{ runtimeMap[record.id].version }}
                </span>
                <span v-else class="text-muted">—</span>
              </div>
            </div>
            <div class="stat-item">
              <div class="stat-item-label">设备 ID</div>
              <div class="stat-item-value vs-id-value">
                <a-tooltip :content="record.id">
                  <span class="cell-ellipsis mono-cell">{{ record.id }}</span>
                </a-tooltip>
              </div>
            </div>
          </div>

          <div v-if="record.points?.length" class="vs-device-points-preview">
            <div
              v-for="(pt, idx) in record.points.slice(0, 3)"
              :key="idx"
              class="expand-point-row"
            >
              <a-tag :color="pt.mode === 'formula' ? 'arcoblue' : 'green'" size="small">
                {{ pt.mode === 'formula' ? '计算' : '映射' }}
              </a-tag>
              <span class="ep-id">{{ pt.point_id }}</span>
              <span class="ep-name">{{ pt.name || '—' }}</span>
              <span v-if="runtimePointValue(record.id, pt.point_id)" class="ep-value">
                = {{ formatValue(runtimePointValue(record.id, pt.point_id)) }}
              </span>
            </div>
            <div v-if="record.points.length > 3" class="vs-more-points">
              还有 {{ record.points.length - 3 }} 个点位
            </div>
          </div>

          <div class="vs-device-card-actions">
            <a-button type="text" size="mini" @click="openDetail(record)">查看值</a-button>
            <a-button type="text" size="mini" @click="openBuilder(record)">编辑</a-button>
            <a-popconfirm content="确定删除该虚拟设备？" @ok="removeDevice(record.id)">
              <a-button type="text" size="mini" status="danger">删除</a-button>
            </a-popconfirm>
          </div>
        </div>
      </div>

      <div v-else class="table-container saas-table">
        <a-table
          class="virtual-shadow-table"
          :columns="columns"
          :data="devices"
          row-key="id"
          size="small"
          :bordered="false"
          :pagination="{ showTotal: true }"
          :expandable="expandable"
          :scroll="{ x: 960 }"
        >
          <template #id="{ record }">
            <a-tooltip :content="record.id">
              <span class="cell-ellipsis mono-cell">{{ record.id }}</span>
            </a-tooltip>
          </template>
          <template #name="{ record }">
            <a-tooltip :content="nameTooltip(record)">
              <span class="cell-ellipsis">{{ displayName(record) }}</span>
            </a-tooltip>
          </template>
          <template #enable="{ record }">
            <span class="table-cell-semantic">
              <a-tag :color="record.enable ? 'green' : 'gray'" size="small" bordered>
                {{ record.enable ? '启用' : '禁用' }}
              </a-tag>
            </span>
          </template>
          <template #points="{ record }">
            <a-tooltip :content="`${record.points?.length || 0} 个点位`">
              <span class="table-cell-count">{{ record.points?.length || 0 }}</span>
            </a-tooltip>
          </template>
          <template #runtime="{ record }">
            <span class="table-cell-semantic">
              <span v-if="runtimeMap[record.id]" class="runtime-badge">
                v{{ runtimeMap[record.id].version }}
              </span>
              <span v-else class="text-muted">—</span>
            </span>
          </template>
          <template #ops="{ record }">
            <div class="table-ops">
              <a-button type="text" size="small" @click="openDetail(record)">查看值</a-button>
              <a-button type="text" size="small" @click="openBuilder(record)">编辑</a-button>
              <a-popconfirm content="确定删除该虚拟设备？" @ok="removeDevice(record.id)">
                <a-button type="text" size="small" status="danger">删除</a-button>
              </a-popconfirm>
            </div>
          </template>
          <template #expand-row="{ record }">
            <div class="expand-points">
              <div
                v-for="(pt, idx) in record.points || []"
                :key="idx"
                class="expand-point-row"
              >
                <a-tag :color="pt.mode === 'formula' ? 'arcoblue' : 'green'" size="small">
                  {{ pt.mode === 'formula' ? '计算' : '映射' }}
                </a-tag>
                <span class="ep-id">{{ pt.point_id }}</span>
                <span class="ep-name">{{ pt.name || '—' }}</span>
                <code class="ep-expr">{{ pointExpr(pt) }}</code>
                <span v-if="runtimePointValue(record.id, pt.point_id)" class="ep-value">
                  = {{ formatValue(runtimePointValue(record.id, pt.point_id)) }}
                </span>
              </div>
            </div>
          </template>
        </a-table>
      </div>
    </a-spin>

    <!-- 积木编辑器 -->
    <a-modal
      v-model:visible="builderVisible"
      :title="editingId ? '编辑虚拟影子设备' : '新建虚拟影子设备'"
      :width="1140"
      :style="{ maxWidth: 'calc(100vw - 48px)' }"
      modal-class="virtual-shadow-builder-modal"
      :mask-closable="false"
      unmount-on-close
      ok-text="保存"
      @before-ok="saveDevice"
      @cancel="closeBuilder"
    >
      <div class="builder-form form-controls-md" :class="{ 'is-dragging': dragState.active }">
        <!-- 拖拽进行中全局提示 -->
        <div v-if="dragState.active" class="drag-floating-badge">
          <icon-drag-dot-vertical />
          <span>{{ dragState.count > 1 ? `拖拽 ${dragState.count} 个点位` : dragState.label }}</span>
          <span class="drag-arrow">→ 放入映射区</span>
        </div>
        <div class="builder-meta">
          <a-row :gutter="12" align="end" class="builder-meta-row" :wrap="false">
            <a-col flex="180px">
              <div class="form-field builder-meta-id">
                <div class="field-label">设备 ID <span class="req">*</span></div>
                <a-input
                  v-model="form.id"
                  :disabled="!!editingId"
                  placeholder="例如 v1"
                  :status="idError ? 'error' : undefined"
                />
                <div v-if="idError" class="field-error">{{ idError }}</div>
              </div>
            </a-col>
            <a-col flex="1" class="builder-meta-name">
              <div class="form-field">
                <div class="field-label">名称</div>
                <a-input v-model="form.name" placeholder="Modbus 从站 1-虚拟" />
              </div>
            </a-col>
            <a-col flex="1" class="builder-meta-desc">
              <div class="form-field">
                <div class="field-label">描述</div>
                <a-input v-model="form.description" placeholder="可选说明" />
              </div>
            </a-col>
            <a-col flex="none" class="builder-meta-enable">
              <div class="form-field enable-field enable-field--inline">
                <div class="field-label">启用</div>
                <a-switch v-model="form.enable" />
              </div>
            </a-col>
          </a-row>
        </div>

        <div class="builder-split">
          <!-- 源设备 / 点位选择 -->
          <div class="source-panel">
            <template v-if="!selectedSourceDevice">
              <div class="panel-title"><icon-search /> 选择源设备</div>
              <div class="panel-hint">先选通道加载设备列表，可再输入名称进一步筛选</div>
              <div
                v-if="showMacBuilderHint"
                class="vs-mac-hint"
                :class="{ 'is-fading': macHintFading }"
              >
                <div class="vs-mac-hint__anim" aria-hidden="true">
                  <div class="vs-mac-hint__scene">
                    <div class="vs-mac-hint__card">
                      <span class="vs-mac-hint__grip">⋮⋮</span>
                      <span class="vs-mac-hint__card-line" />
                    </div>
                    <div class="vs-mac-hint__cursor">
                      <svg viewBox="0 0 24 24" width="14" height="14" fill="currentColor">
                        <path d="M5.5 3.21l10.89 6.9a.5.5 0 010 .84L5.5 17.84a.5.5 0 01-.79-.4V3.61a.5.5 0 01.79-.4z" />
                      </svg>
                    </div>
                    <div class="vs-mac-hint__arrow">→</div>
                    <div class="vs-mac-hint__target">映射区</div>
                  </div>
                </div>
                <div class="vs-mac-hint__body">
                  <div class="vs-mac-hint__title">Mac 操作提示</div>
                  <p class="vs-mac-hint__text">
                    <strong>点击</strong>设备进入选点；
                    <strong>按住并拖动</strong>设备或点位至右侧映射区（触控板可用）；
                    也可点「批量映射」快速添加
                  </p>
                </div>
                <button type="button" class="vs-mac-hint__close" aria-label="关闭提示" @click="dismissMacHint">
                  ×
                </button>
              </div>
              <div class="field-stack">
                <div class="form-field">
                  <div class="field-label-sm">源设备通道</div>
                  <a-select
                    v-model="sourceChannelId"
                    placeholder="选择通道以加载设备"
                    allow-clear
                    allow-search
                  >
                    <a-option v-for="ch in channels" :key="ch.id" :value="ch.id">
                      {{ ch.name }} ({{ ch.id }})
                    </a-option>
                  </a-select>
                </div>
                <div class="form-field">
                  <div class="field-label-sm">筛选设备</div>
                  <a-input-search
                    v-model="deviceSearch"
                    placeholder="名称 / ID（可选）"
                    allow-clear
                    :loading="deviceSearchLoading"
                    :disabled="!sourceChannelId"
                    search-button
                    @search="searchSourceDevices"
                    @press-enter="searchSourceDevices"
                    @clear="onDeviceSearchClear"
                  />
                </div>
              </div>
              <a-spin :loading="deviceSearchLoading" class="device-list-spin">
                <div class="device-list">
                  <template v-if="!sourceChannelId">
                    <div class="search-placeholder">
                      <icon-search :size="28" />
                      <p>请先选择通道</p>
                      <span>选择后将自动列出该通道下全部设备</span>
                    </div>
                  </template>
                  <template v-else-if="deviceSearchResults.length">
                    <div class="search-result-tip">
                      {{ deviceSearch.trim() ? '筛选到' : '共' }} {{ deviceSearchResults.length }} 台设备
                    </div>
                    <div
                      v-for="dev in deviceSearchResults"
                      :key="dev.key"
                      class="device-card"
                      :class="{
                        'is-dragging': draggingDeviceKey === dev.key,
                        'is-press-hint': devicePressHint.key === dev.key,
                        'is-pressing': devicePressState.key === dev.key && devicePressState.pressing
                      }"
                      :draggable="!dragUsesPointer"
                      @click="onDeviceCardClick(dev)"
                      @pointerdown="onDeviceCardPointerDown($event, dev)"
                      @pointerup="onDeviceCardPressEnd"
                      @pointerleave="onDeviceCardPressEnd"
                      @pointercancel="onDeviceCardPressEnd"
                      @dragstart="onDeviceDragStart($event, dev)"
                      @dragend="onDragEnd"
                    >
                      <span class="device-drag-grip" aria-hidden="true">
                        <icon-drag-dot-vertical />
                      </span>
                      <div class="device-card-body">
                        <div class="device-card-main">
                          <span class="device-name" v-html="highlightMatch(dev.device_name || dev.name || dev.device_id || '-', deviceSearch)" />
                          <a-tag size="small" color="arcoblue">{{ dev.point_count ?? dev.pointCount ?? 0 }} 点</a-tag>
                        </div>
                        <div class="device-card-sub">
                          <span>{{ dev.channel_name || dev.channel_id || '-' }}</span>
                          <span class="device-id" v-html="'ID: ' + highlightMatch(dev.device_id || dev.id || '-', deviceSearch)" />
                        </div>
                      </div>
                      <a-button
                        v-if="dragUsesPointer"
                        type="text"
                        size="mini"
                        class="device-quick-add"
                        @click.stop="quickAddDevice(dev)"
                      >
                        批量映射
                      </a-button>
                      <div
                        v-if="devicePressHint.key === dev.key"
                        class="device-drag-hint"
                        :class="{ visible: devicePressHint.visible }"
                      >
                        <icon-drag-dot-vertical />
                        <span>拖拽到右侧映射区</span>
                      </div>
                    </div>
                  </template>
                  <a-empty v-else-if="deviceSearchDone" description="该通道下无匹配设备" />
                </div>
              </a-spin>
            </template>

            <template v-else>
              <div class="point-picker-header">
                <a-button type="text" size="mini" @click="clearSourceDevice">
                  <template #icon><icon-left /></template>
                  返回检索
                </a-button>
                <div class="point-picker-title">
                  <div class="device-name">
                    {{ selectedSourceDevice.device_name }}
                    <a-tag
                      v-if="selectedSourceDeviceOnline"
                      size="small"
                      color="green"
                      bordered
                    >
                      在线
                    </a-tag>
                    <a-tag v-else size="small" color="gray" bordered>离线</a-tag>
                  </div>
                  <div class="device-card-sub">{{ selectedSourceDevice.channel_name }} · {{ selectedSourceDevice.device_id }}</div>
                </div>
              </div>
              <div class="point-picker-toolbar">
                <a-checkbox
                  :model-value="isAllDevicePointsSelected"
                  :indeterminate="isDevicePointsIndeterminate"
                  @change="toggleSelectAllDevicePoints"
                >
                  全选
                </a-checkbox>
                <span v-if="selectedPointRefs.size" class="sel-count">已选 {{ selectedPointRefs.size }}</span>
                <a-button
                  v-if="selectedPointRefs.size"
                  size="mini"
                  type="primary"
                  @click="batchAddSelectedToMapping"
                >
                  批量添加
                </a-button>
              </div>
              <div class="form-field point-filter-field">
                <a-input-search
                  v-model="pointFilter"
                  placeholder="过滤点位名称 / ID"
                  allow-clear
                />
              </div>
              <a-spin :loading="pointsLoading" class="point-list-spin">
                <div
                  class="point-list"
                  :class="{ 'drag-over': pointListDragOver }"
                  @dragover.prevent="pointListDragOver = true"
                  @dragleave="pointListDragOver = false"
                >
                  <template v-if="pointsLoading && filteredDevicePoints.length === 0">
                    <div v-for="n in 6" :key="n" class="point-chip point-chip-skeleton">
                      <div class="point-chip-body">
                        <span class="point-chip-id skeleton-line" />
                        <span class="point-chip-sub skeleton-line skeleton-line--short" />
                      </div>
                    </div>
                  </template>
                  <div
                    v-for="src in filteredDevicePoints"
                    :key="src.ref"
                    class="point-chip"
                    :class="{
                      selected: selectedPointRefs.has(src.ref),
                      'is-dragging': draggingRefs.has(src.ref)
                    }"
                    @click="togglePointSelection(src.ref)"
                  >
                    <span
                      class="drag-grip"
                      title="拖拽"
                      :draggable="!dragUsesPointer"
                      @pointerdown.stop="onPointPointerDown($event, src)"
                      @dragstart="onPointDragStart($event, src)"
                      @dragend="onDragEnd"
                      @click.stop
                    >
                      <icon-drag-dot-vertical />
                    </span>
                    <a-checkbox
                      :model-value="selectedPointRefs.has(src.ref)"
                      @mousedown.stop
                      @click.stop
                      @change="togglePointSelection(src.ref)"
                    />
                    <div
                      class="point-chip-body point-chip-drag"
                      :draggable="!dragUsesPointer"
                      @pointerdown.stop="onPointPointerDown($event, src)"
                      @dragstart="onPointDragStart($event, src)"
                      @dragend="onDragEnd"
                      @click.stop
                    >
                      <span class="point-chip-id">{{ src.point_name || src.point_id }}</span>
                      <span class="point-chip-sub">{{ src.point_id }}</span>
                    </div>
                    <span v-if="sourceValue(src.ref)" class="point-chip-val">
                      {{ formatValue(sourceValue(src.ref)) }}
                    </span>
                    <span
                      v-else-if="selectedSourceDeviceOnline && valuesLoading"
                      class="point-chip-val point-chip-val--loading"
                    >
                      …
                    </span>
                  </div>
                  <a-empty v-if="filteredDevicePoints.length === 0 && !pointsLoading" description="无匹配点位" />
                </div>
              </a-spin>
              <div
                v-if="selectedPointRefs.size"
                class="batch-drag-handle"
                :class="{ 'is-dragging': dragState.active && dragState.count > 1 }"
                :draggable="!dragUsesPointer"
                @pointerdown="onBatchPointerDown"
                @dragstart="onBatchDragStart"
                @dragend="onDragEnd"
              >
                <icon-drag-dot-vertical />
                拖拽已选 {{ selectedPointRefs.size }} 个点位到右侧映射区
              </div>
            </template>
          </div>

          <!-- 虚拟点位积木 -->
          <div class="points-panel">
            <div class="panel-title-row">
              <span class="panel-title"><icon-apps /> 虚拟点位</span>
              <a-button size="mini" type="outline" @click="addPoint('map')">+ 映射块</a-button>
              <a-button size="mini" type="outline" status="success" @click="addPoint('formula')">+ 计算块</a-button>
              <a-dropdown @select="applyFormulaTemplate">
                <a-button size="mini" type="text">公式模板</a-button>
                <template #content>
                  <a-doption value="sum">两路求和 (a + b)</a-doption>
                  <a-doption value="diff">两路差值 (a - b)</a-doption>
                  <a-doption value="avg">平均值 ((a + b) / 2)</a-doption>
                  <a-doption value="scale">倍率 (a * 1.5)</a-doption>
                </template>
              </a-dropdown>
              <a-button
                size="mini"
                type="outline"
                :status="duplicatePointIdCount ? 'warning' : undefined"
                @click="locateDuplicatePointIds"
              >
                查重{{ duplicatePointIdCount ? ` (${duplicatePointIdCount})` : '' }}
              </a-button>
            </div>

            <!-- 批量映射投放区 -->
            <div
              class="batch-drop-canvas"
              :class="{ 'drop-active': batchDropActive, 'drop-idle-hint': dragState.active && !batchDropActive }"
              @dragover.prevent="onBatchZoneDragOver"
              @dragleave="onBatchZoneDragLeave"
              @drop.prevent="onBatchZoneDrop"
            >
              <div v-if="batchDropActive" class="drop-release-hint">
                <icon-check-circle /> 松开鼠标，添加 {{ dragState.count || 1 }} 个映射
              </div>
              <div class="batch-drop-inner" :class="{ dimmed: batchDropActive }">
                <icon-drag-dot-vertical class="batch-drop-icon" />
                <div class="batch-drop-title">批量映射区</div>
                <div class="batch-drop-hint">
                  将左侧单个或多个点位拖入此处，自动创建映射积木（已存在的来源会自动跳过）
                </div>
                <div v-if="form.points.length" class="batch-drop-stat">
                  当前 {{ form.points.length }} 个虚拟点位 · 其中 {{ mapPointCount }} 个映射
                </div>
              </div>
            </div>

            <div v-if="form.points.length === 0" class="empty-blocks-hint">
              拖入点位到上方批量区，或点击「+ 映射块」手动添加
            </div>

            <div class="blocks-scroll">
              <div
                v-for="(pt, idx) in form.points"
                :key="`${idx}-${pt.point_id}-${pt.source_ref}`"
                class="point-block"
                :class="{
                  active: activePointIndex === idx,
                  'drop-hover': dropHoverIndex === idx,
                  'duplicate-id': isDuplicatePointId(idx)
                }"
                @click="activePointIndex = idx"
                @dragover.prevent="onBlockDragOver(idx, $event)"
                @dragleave="onBlockDragLeave"
                @drop.prevent="onBlockDrop($event, idx)"
              >
                <div class="point-block-header">
                  <span class="block-badge" :class="pt.mode">{{ pt.mode === 'map' ? '映射' : '计算' }}</span>
                  <span class="block-index">#{{ idx + 1 }}</span>
                  <a-radio-group v-model="pt.mode" type="button" size="mini" @click.stop>
                    <a-radio value="map">直接映射</a-radio>
                    <a-radio value="formula">公式计算</a-radio>
                  </a-radio-group>
                  <a-space size="mini" class="block-actions">
                    <a-button type="text" size="mini" :disabled="idx === 0" @click.stop="movePoint(idx, -1)">↑</a-button>
                    <a-button type="text" size="mini" :disabled="idx === form.points.length - 1" @click.stop="movePoint(idx, 1)">↓</a-button>
                    <a-button type="text" size="mini" status="danger" @click.stop="removePoint(idx)">删除</a-button>
                  </a-space>
                </div>

                <a-row :gutter="8" class="mb-2">
                  <a-col :span="8">
                    <a-input
                      v-model="pt.point_id"
                      placeholder="虚拟点位 ID"
                      :status="isDuplicatePointId(idx) ? 'error' : undefined"
                      @click.stop
                    />
                    <div v-if="isDuplicatePointId(idx)" class="field-error field-error--inline">
                      虚拟点位 ID 不得重复
                    </div>
                  </a-col>
                  <a-col :span="8">
                    <a-input v-model="pt.name" placeholder="显示名称" @click.stop />
                  </a-col>
                  <a-col :span="8">
                    <a-input v-model="pt.unit" placeholder="单位" @click.stop />
                  </a-col>
                </a-row>

                <div v-if="pt.mode === 'map'" class="map-mode">
                  <div class="field-label-sm">映射来源</div>
                  <div
                    class="map-drop-zone"
                    :class="{ filled: !!pt.source_ref, 'drop-hover': mapDropHoverIndex === idx }"
                    @dragover.prevent.stop="onMapZoneDragOver(idx, $event)"
                    @dragleave.stop="onMapZoneDragLeave"
                    @drop.prevent.stop="onMapZoneDrop($event, idx)"
                  >
                    <template v-if="pt.source_ref">
                      <a-tag color="arcoblue" closable @close="pt.source_ref = ''">
                        {{ pt.source_ref }}
                      </a-tag>
                      <span v-if="sourceValue(pt.source_ref)" class="inline-live">
                        {{ formatValue(sourceValue(pt.source_ref)) }}
                      </span>
                    </template>
                    <span v-else class="drop-placeholder">
                      <icon-drag-dot-vertical v-if="dragState.active" class="drop-icon-bounce" />
                      {{ dragState.active ? '松开放入此映射位' : '拖入 1 个来源点位' }}
                    </span>
                  </div>
                </div>

                <div v-else class="formula-mode">
                  <div class="field-label-sm">计算公式</div>
                  <div class="formula-toolbar">
                    <a-button v-for="op in operators" :key="op" size="mini" @click.stop="insertFormula(op)">{{ op }}</a-button>
                    <a-button size="mini" @click.stop="insertFormula(' ')">空格</a-button>
                  </div>
                  <a-textarea
                    v-model="pt.formula"
                    :auto-size="{ minRows: 2, maxRows: 4 }"
                    placeholder="例如 ch1.dev1.temp + ch1.dev2.temp"
                    @click.stop
                  />
                  <div v-if="formulaDeps(pt.formula).length" class="formula-deps">
                    <span class="deps-label">依赖:</span>
                    <a-tag v-for="dep in formulaDeps(pt.formula)" :key="dep" size="small" color="gray">
                      {{ dep }}
                      <span v-if="sourceValue(dep)" class="dep-val">({{ formatValue(sourceValue(dep)) }})</span>
                    </a-tag>
                  </div>
                  <div class="hint">从左侧拖入点位插入引用；支持 + - * / 和括号</div>
                </div>

                <div v-if="editingId && previewValues[pt.point_id]" class="preview-row">
                  预览值: <strong>{{ formatValue(previewValues[pt.point_id]) }}</strong>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>
    </a-modal>

    <!-- 运行时详情（teleport 到 body，须独立 drawer class） -->
    <a-drawer
      v-model:visible="detailVisible"
      class="virtual-shadow-detail-drawer"
      :title="detailDevice ? `虚拟设备 · ${detailDevice.name || detailDevice.id}` : '虚拟设备'"
      width="520px"
      unmount-on-close
    >
      <template v-if="detailDevice">
        <a-descriptions :column="1" size="small" bordered class="detail-drawer-desc mb-4">
          <a-descriptions-item label="ID">{{ detailDevice.id }}</a-descriptions-item>
          <a-descriptions-item v-if="detailDevice.description" label="描述">{{ detailDevice.description }}</a-descriptions-item>
          <a-descriptions-item label="状态">
            <a-tag :color="detailDevice.enable ? 'green' : 'gray'" size="small" bordered>
              {{ detailDevice.enable ? '启用' : '禁用' }}
            </a-tag>
          </a-descriptions-item>
        </a-descriptions>

        <a-table
          class="virtual-shadow-detail-table"
          :columns="detailColumns"
          :data="detailRows"
          size="small"
          :bordered="false"
          :pagination="false"
          row-key="point_id"
        >
          <template #mode="{ record }">
            <a-tag :color="record.mode === 'formula' ? 'arcoblue' : 'green'" size="small" bordered>
              {{ record.mode === 'formula' ? '计算' : '映射' }}
            </a-tag>
          </template>
          <template #expr="{ record }">
            <a-tooltip :content="record.expr">
              <code class="expr-code cell-ellipsis">{{ record.expr }}</code>
            </a-tooltip>
          </template>
          <template #value="{ record }">
            <span v-if="record.runtime">{{ formatValue(record.runtime) }}</span>
            <span v-else class="text-muted">—</span>
          </template>
        </a-table>

        <div class="drawer-footer">
          <a-button type="outline" size="small" @click="refreshDetailRuntime" :loading="detailLoading">
            <template #icon><icon-refresh /></template>
            刷新实时值
          </a-button>
        </div>
      </template>
    </a-drawer>

    <VirtualShadowHelpDrawer v-model:visible="helpVisible" />
  </div>
</template>

<script setup>
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { Message } from '@arco-design/web-vue'
import {
  IconPlus,
  IconRefresh,
  IconApps,
  IconDragDotVertical,
  IconSearch,
  IconLeft,
  IconCheckCircle,
  IconQuestionCircle
} from '@arco-design/web-vue/es/icon'
import { isMac } from '@/utils/platform'
import VirtualShadowHelpDrawer from '@/components/virtual-shadow/VirtualShadowHelpDrawer.vue'
import request from '@/utils/request'
import {
  createVirtualShadow,
  deleteVirtualShadow,
  fetchSourceValues,
  getVirtualShadow,
  listDevicePointSources,
  listVirtualShadows,
  searchVirtualShadowDevices,
  updateVirtualShadow
} from '@/api/virtualShadow'
import {
  FORMULA_OPERATORS,
  decodeDragPayload,
  encodeDragPayload,
  findDuplicatePointIds,
  flattenDuplicatePointIndices,
  fuzzyMatch,
  makePointRef,
  isDeviceOnline,
  mapDeviceToSummary,
  mapPointToSource,
  normalizeArrayResponse,
  newVirtualDeviceForm,
  newVirtualPoint,
  parsePointRef,
  DRAG_MIME
} from '@/utils/virtualShadowRef'

const route = useRoute()
const router = useRouter()

const ID_PATTERN = /^[a-zA-Z][a-zA-Z0-9_-]{0,63}$/

const loading = ref(false)
const viewMode = ref('list')
const helpVisible = ref(false)
const devices = ref([])
const channels = ref([])
const sourceCache = reactive(new Map())
const runtimeMap = reactive({})
const sourceValueMap = reactive({})
const previewValues = reactive({})

const builderVisible = ref(false)
const editingId = ref('')
const form = reactive(newVirtualDeviceForm())
const sourceChannelId = ref('')
const activePointIndex = ref(0)
const deviceSearch = ref('')
const deviceSearchResults = ref([])
const deviceSearchLoading = ref(false)
const deviceSearchDone = ref(false)
const selectedSourceDevice = ref(null)
const activeDevicePoints = ref([])
const pointsLoading = ref(false)
const valuesLoading = ref(false)
const pointFilter = ref('')
const devicePointsCache = reactive(new Map())
const selectedPointRefs = reactive(new Set())
const dropHoverIndex = ref(-1)
const batchDropActive = ref(false)
const pointListDragOver = ref(false)
const mapDropHoverIndex = ref(-1)

const MAC_HINT_STORAGE_KEY = 'vs_builder_mac_hint_dismissed'
const MAC_HINT_AUTO_FADE_MS = 12000
const isMacPlatform = ref(false)
const macHintVisible = ref(false)
const macHintFading = ref(false)
let macHintFadeTimer = null

const dragState = reactive({
  active: false,
  count: 0,
  label: ''
})
const dragUsesPointer = ref(false)
let dragGhostEl = null
let pointerGhostEl = null
let pointerDragSession = null
let suppressClick = false
const POINTER_DRAG_THRESHOLD = 10
const draggingRefs = reactive(new Set())

function isAppleOrTouchDrag() {
  if (typeof window === 'undefined') return false
  // macOS trackpad reports pointerType "mouse" (fine pointer); HTML5 DnD is unreliable there
  if (isMac()) return true
  if ('ontouchstart' in window || navigator.maxTouchPoints > 0) return true
  if (window.matchMedia?.('(pointer: coarse)')?.matches) return true
  return false
}

const showMacBuilderHint = computed(
  () =>
    builderVisible.value &&
    isMacPlatform.value &&
    macHintVisible.value &&
    !selectedSourceDevice.value
)

function clearMacHintFadeTimer() {
  if (macHintFadeTimer) {
    clearTimeout(macHintFadeTimer)
    macHintFadeTimer = null
  }
}

function dismissMacHint(persist = true) {
  clearMacHintFadeTimer()
  macHintFading.value = true
  window.setTimeout(() => {
    macHintVisible.value = false
    macHintFading.value = false
    if (persist) {
      try {
        localStorage.setItem(MAC_HINT_STORAGE_KEY, '1')
      } catch (_) {
        /* ignore */
      }
    }
  }, 320)
}

function scheduleMacHintAutoFade() {
  clearMacHintFadeTimer()
  if (!macHintVisible.value || !isMacPlatform.value) return
  macHintFadeTimer = window.setTimeout(() => dismissMacHint(true), MAC_HINT_AUTO_FADE_MS)
}

function deviceDragPayload(dev) {
  const refs = devicePointsPrefetch.get(dev.key) || []
  return {
    refs,
    device: {
      key: dev.key,
      channel_id: dev.channel_id,
      device_id: dev.device_id,
      device_name: dev.device_name,
      point_count: dev.point_count
    },
    label: `${dev.device_name} · ${dev.point_count} 点`
  }
}

function pointDragPayload(src) {
  const refs = refsForDrag(src)
  return {
    refs,
    device: null,
    label: src.point_name || src.point_id
  }
}

function batchDragPayload() {
  const refs = [...selectedPointRefs]
  return {
    refs,
    device: null,
    label: `${refs.length} 个点位`
  }
}

function onDeviceCardPointerDown(e, dev) {
  if (e.button !== 0) return
  onDeviceCardPressStart(dev)
  if (dragUsesPointer.value) beginPointerDragSession(e, deviceDragPayload(dev))
}

function onPointPointerDown(e, src) {
  if (e.button !== 0) return
  if (dragUsesPointer.value) beginPointerDragSession(e, pointDragPayload(src))
}

function onBatchPointerDown(e) {
  if (e.button !== 0 || !selectedPointRefs.size) return
  if (dragUsesPointer.value) beginPointerDragSession(e, batchDragPayload())
}

async function quickAddDevice(dev) {
  await handleDeviceDrop({
    key: dev.key,
    channel_id: dev.channel_id,
    device_id: dev.device_id,
    device_name: dev.device_name,
    point_count: dev.point_count
  })
}

function beginPointerDragSession(e, payload) {
  endPointerDragSession()
  pointerDragSession = {
    pointerId: e.pointerId,
    startX: e.clientX,
    startY: e.clientY,
    active: false,
    payload,
    captureEl: e.currentTarget
  }
  pointerDragSession.captureEl?.setPointerCapture?.(e.pointerId)
  window.addEventListener('pointermove', onWindowPointerMove, { passive: false })
  window.addEventListener('pointerup', onWindowPointerUp)
  window.addEventListener('pointercancel', onWindowPointerUp)
}

function activatePointerDrag(session) {
  session.active = true
  deviceDidDrag = true
  suppressClick = true
  const { refs, device, label } = session.payload
  dragState.active = true
  dragState.count = refs.length || device?.point_count || 1
  dragState.label = label || refs[0] || device?.device_name || '点位'
  draggingRefs.clear()
  for (const r of refs) draggingRefs.add(r)
  if (device) draggingDeviceKey.value = device.key
  clearDevicePressHint()

  if (pointerGhostEl) {
    pointerGhostEl.remove()
    pointerGhostEl = null
  }
  const ghost = document.createElement('div')
  ghost.className = 'vs-drag-ghost vs-pointer-ghost' + (device ? ' vs-drag-ghost--device' : '')
  const icon = document.createElement('span')
  icon.className = 'vs-drag-ghost-icon'
  icon.textContent = '⋮⋮'
  const text = document.createElement('span')
  text.className = 'vs-drag-ghost-text'
  text.textContent =
    device && refs.length > 1
      ? `${device.device_name} · ${refs.length} 点`
      : device
        ? `${device.device_name} · ${device.point_count || refs.length || 0} 点`
        : refs.length > 1
          ? `${refs.length} 个点位`
          : (label || refs[0] || '')
  ghost.appendChild(icon)
  ghost.appendChild(text)
  document.body.appendChild(ghost)
  pointerGhostEl = ghost
  movePointerGhost(session.startX, session.startY)
}

function movePointerGhost(x, y) {
  if (!pointerGhostEl) return
  pointerGhostEl.style.left = `${x}px`
  pointerGhostEl.style.top = `${y}px`
  pointerGhostEl.style.transform = 'translate(-50%, -50%)'
}

function findDropTargetAt(x, y) {
  const modal = document.querySelector('.virtual-shadow-builder-modal')
  if (!modal) return null
  const el = document.elementFromPoint(x, y)
  if (!el || !modal.contains(el)) return null

  const mapEl = el.closest('.map-drop-zone')
  if (mapEl) {
    const blockEl = mapEl.closest('.point-block')
    const blocks = modal.querySelectorAll('.point-block')
    const idx = Array.from(blocks).indexOf(blockEl)
    if (idx >= 0) return { type: 'map', idx }
  }

  const batchEl = el.closest('.batch-drop-canvas')
  if (batchEl) return { type: 'batch' }

  const blockEl = el.closest('.point-block')
  if (blockEl) {
    const blocks = modal.querySelectorAll('.point-block')
    const idx = Array.from(blocks).indexOf(blockEl)
    if (idx >= 0) return { type: 'block', idx }
  }
  return null
}

function updateDropHoverFromPoint(x, y) {
  const target = findDropTargetAt(x, y)
  batchDropActive.value = target?.type === 'batch'
  dropHoverIndex.value = target?.type === 'block' ? target.idx : -1
  mapDropHoverIndex.value = target?.type === 'map' ? target.idx : -1
}

function onWindowPointerMove(e) {
  if (!pointerDragSession || e.pointerId !== pointerDragSession.pointerId) return
  const dx = e.clientX - pointerDragSession.startX
  const dy = e.clientY - pointerDragSession.startY
  if (!pointerDragSession.active) {
    if (Math.hypot(dx, dy) < POINTER_DRAG_THRESHOLD) return
    activatePointerDrag(pointerDragSession)
  }
  e.preventDefault()
  movePointerGhost(e.clientX, e.clientY)
  updateDropHoverFromPoint(e.clientX, e.clientY)
}

async function onWindowPointerUp(e) {
  if (!pointerDragSession || e.pointerId !== pointerDragSession.pointerId) return
  const session = pointerDragSession
  const wasActive = session.active
  const target = wasActive ? findDropTargetAt(e.clientX, e.clientY) : null
  endPointerDragSession()
  if (wasActive) {
    await handlePointerDrop(target, session.payload)
    nextTick(() => {
      suppressClick = false
    })
  }
}

function endPointerDragSession() {
  window.removeEventListener('pointermove', onWindowPointerMove)
  window.removeEventListener('pointerup', onWindowPointerUp)
  window.removeEventListener('pointercancel', onWindowPointerUp)
  if (pointerDragSession?.captureEl && pointerDragSession.pointerId != null) {
    try {
      pointerDragSession.captureEl.releasePointerCapture(pointerDragSession.pointerId)
    } catch (_) {
      /* ignore */
    }
  }
  pointerDragSession = null
  if (pointerGhostEl) {
    pointerGhostEl.remove()
    pointerGhostEl = null
  }
}

async function handlePointerDrop(target, payload) {
  clearDragVisualState()
  if (!target) return
  if (target.type === 'batch') {
    await applyBatchDropPayload(payload)
  } else if (target.type === 'map') {
    applyMapDropPayload(payload, target.idx)
  } else if (target.type === 'block') {
    applyBlockDropPayload(payload, target.idx)
  }
}

const draggingDeviceKey = ref(null)
const devicePressHint = reactive({ visible: false, key: '' })
const devicePressState = reactive({ key: '', pressing: false })
const devicePointsPrefetch = reactive(new Map())
let devicePressTimer = null
let deviceDidDrag = false
const DEVICE_PRESS_HINT_MS = 420

const detailVisible = ref(false)
const detailDevice = ref(null)
const detailRuntime = ref(null)
const detailLoading = ref(false)

const operators = FORMULA_OPERATORS

const columns = [
  { title: 'ID', slotName: 'id', dataIndex: 'id', width: 140, ellipsis: true, tooltip: true },
  { title: '名称', slotName: 'name', dataIndex: 'name', width: 168, ellipsis: true, tooltip: true },
  { title: '点位', slotName: 'points', width: 64, align: 'center' },
  { title: '启用', slotName: 'enable', width: 88 },
  { title: '运行时', slotName: 'runtime', width: 96 },
  { title: '操作', slotName: 'ops', width: 220, fixed: 'right' }
]

function displayName(record) {
  const name = (record?.name || '').trim()
  return name || record?.id || '—'
}

function nameTooltip(record) {
  const name = (record?.name || '').trim()
  const id = record?.id || ''
  if (name && name !== id) return `${name}\nID: ${id}`
  return name || id || '—'
}

const detailColumns = [
  { title: '点位', dataIndex: 'point_id', width: 100, ellipsis: true, tooltip: true },
  { title: '模式', slotName: 'mode', width: 72 },
  { title: '表达式', slotName: 'expr', ellipsis: true },
  { title: '当前值', slotName: 'value', width: 100 }
]

const expandable = {
  width: 40
}

const idError = computed(() => {
  if (!form.id) return ''
  if (!ID_PATTERN.test(form.id.trim())) {
    return '字母开头，仅含字母数字 _ -，最长 64 字符'
  }
  return ''
})


function cacheSources(list) {
  for (const s of list || []) {
    if (s?.ref) sourceCache.set(s.ref, s)
  }
}

function resolveSource(ref) {
  if (sourceCache.has(ref)) return sourceCache.get(ref)
  const parsed = parsePointRef(ref)
  if (!parsed) return null
  return {
    ref,
    channel_id: parsed.channelId,
    device_id: parsed.deviceId,
    point_id: parsed.pointId,
    point_name: parsed.pointId,
    device_name: parsed.deviceId,
    channel_name: parsed.channelId
  }
}

function highlightMatch(text, query) {
  const src = String(text || '')
  const q = String(query || '').trim()
  if (!q) return escapeHtml(src)
  const lower = src.toLowerCase()
  const tokens = q.toLowerCase().split(/\s+/).filter(Boolean)
  let html = escapeHtml(src)
  for (const token of tokens) {
    const idx = lower.indexOf(token)
    if (idx >= 0) {
      const orig = src.substring(idx, idx + token.length)
      html = html.replace(orig, `<mark>${escapeHtml(orig)}</mark>`)
    }
  }
  return html
}

function escapeHtml(s) {
  return String(s)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
}

let deviceSearchTimer = null
watch(deviceSearch, () => {
  clearTimeout(deviceSearchTimer)
  if (!sourceChannelId.value) return
  deviceSearchTimer = setTimeout(() => searchSourceDevices(), 350)
})

watch(
  () => sourceChannelId.value,
  (channelId, prev) => {
    if (!builderVisible.value) return
    if (channelId === prev) return
    onSourceChannelChange()
  }
)

watch(
  () => builderVisible.value,
  visible => {
    if (visible && isMacPlatform.value && macHintVisible.value) {
      scheduleMacHintAutoFade()
    } else {
      clearMacHintFadeTimer()
    }
  }
)

watch(
  () => dragState.active,
  active => {
    if (active && macHintVisible.value) dismissMacHint(true)
  }
)

const filteredDevicePoints = computed(() => {
  const q = pointFilter.value.trim()
  if (!q) return activeDevicePoints.value
  return activeDevicePoints.value.filter(src => {
    const hay = `${src.point_id} ${src.point_name || ''} ${src.ref}`
    return fuzzyMatch(hay, q)
  })
})

const isAllDevicePointsSelected = computed(() => {
  const pts = filteredDevicePoints.value
  return pts.length > 0 && pts.every(p => selectedPointRefs.has(p.ref))
})

const isDevicePointsIndeterminate = computed(() => {
  const pts = filteredDevicePoints.value
  if (!pts.length) return false
  const n = pts.filter(p => selectedPointRefs.has(p.ref)).length
  return n > 0 && n < pts.length
})

const mapPointCount = computed(() => form.points.filter(p => p.mode === 'map').length)

const duplicatePointIdMap = computed(() => findDuplicatePointIds(form.points))

const duplicatePointIdCount = computed(() => duplicatePointIdMap.value.size)

const duplicateLocateCursor = ref(0)

function isDuplicatePointId(idx) {
  const id = form.points[idx]?.point_id?.trim()
  if (!id) return false
  const indices = duplicatePointIdMap.value.get(id)
  return !!(indices && indices.length > 1)
}

function duplicatePointIdSummary() {
  return [...duplicatePointIdMap.value.entries()]
    .map(([id, indices]) => `「${id}」×${indices.length}`)
    .join('、')
}

function scrollToPointBlock(idx) {
  nextTick(() => {
    const modal = document.querySelector('.virtual-shadow-builder-modal')
    const blocks = modal?.querySelectorAll('.point-block')
    blocks?.[idx]?.scrollIntoView({ behavior: 'smooth', block: 'center' })
  })
}

function focusNextDuplicatePointBlock({ announce = true } = {}) {
  const dupes = duplicatePointIdMap.value
  if (dupes.size === 0) {
    if (announce) {
      Message.success('未发现重复的虚拟点位 ID')
      duplicateLocateCursor.value = 0
    }
    return false
  }
  const indices = flattenDuplicatePointIndices(dupes)
  const idx = indices[duplicateLocateCursor.value % indices.length]
  duplicateLocateCursor.value = (duplicateLocateCursor.value + 1) % indices.length
  activePointIndex.value = idx
  scrollToPointBlock(idx)
  if (announce) {
    Message.warning(`虚拟点位 ID 不得重复：${duplicatePointIdSummary()}（已定位 #${idx + 1}）`)
  }
  return true
}

function locateDuplicatePointIds() {
  focusNextDuplicatePointBlock({ announce: true })
}

function warnDuplicatePointIdsIfAny() {
  if (duplicatePointIdMap.value.size === 0) return
  Message.warning(`部分虚拟点位 ID 重复，请修改后再保存：${duplicatePointIdSummary()}`)
}

const selectedSourceDeviceOnline = computed(() => isDeviceOnline(selectedSourceDevice.value))

const detailRows = computed(() => {
  if (!detailDevice.value) return []
  const pts = detailDevice.value.points || []
  const runtimePts = detailRuntime.value?.points || {}
  return pts.map(pt => ({
    point_id: pt.point_id,
    mode: pt.mode,
    expr: pointExpr(pt),
    runtime: runtimePts[pt.point_id]
  }))
})

function pointExpr(pt) {
  if (!pt) return ''
  return pt.mode === 'formula' ? (pt.formula || '—') : (pt.source_ref || '—')
}

function sourceValue(ref) {
  return ref ? sourceValueMap[ref] : null
}

function formatValue(info) {
  if (info == null) return '—'
  if (typeof info === 'object' && 'value' in info) {
    const v = info.value
    if (v == null) return '—'
    if (typeof v === 'number') return Number.isInteger(v) ? String(v) : v.toFixed(3)
    return String(v)
  }
  return String(info)
}

function runtimePointValue(deviceId, pointId) {
  const rt = runtimeMap[deviceId]
  return rt?.points?.[pointId]
}

function formulaDeps(formula) {
  if (!formula) return []
  const deps = new Set()
  const re = /[a-zA-Z][a-zA-Z0-9_-]*(?:\.[a-zA-Z][a-zA-Z0-9_-]*){2,}/g
  let m
  while ((m = re.exec(formula)) !== null) {
    deps.add(m[0])
  }
  return [...deps]
}

/** Infer primary source channel for builder picker prefill from point refs. */
function inferSourceChannelFromPoints() {
  for (const p of form.points) {
    if (p.mode === 'map' && p.source_ref?.trim()) {
      const parsed = parsePointRef(p.source_ref.trim())
      if (parsed?.channelId) return parsed.channelId
    }
    if (p.mode === 'formula' && p.formula) {
      for (const dep of formulaDeps(p.formula)) {
        const parsed = parsePointRef(dep)
        if (parsed?.channelId) return parsed.channelId
      }
    }
  }
  if (channels.value.length === 1) return channels.value[0].id
  return ''
}

async function loadSourceValueMap(sourceList) {
  const list = sourceList || [...sourceCache.values()]
  if (!list.length) return
  valuesLoading.value = true
  try {
    const map = await fetchSourceValues(list)
    Object.assign(sourceValueMap, map)
  } finally {
    valuesLoading.value = false
  }
}

function resolveDeviceSummary(channelId, deviceId) {
  if (
    selectedSourceDevice.value?.device_id === deviceId &&
    selectedSourceDevice.value?.channel_id === channelId
  ) {
    return selectedSourceDevice.value
  }
  return deviceSearchResults.value.find(
    d => d.channel_id === channelId && d.device_id === deviceId
  )
}

async function fetchDevicePointSources(channelId, deviceId) {
  const ch = channels.value.find(c => c.id === channelId)
  const channelName = ch?.name || channelId
  const devSummary = resolveDeviceSummary(channelId, deviceId)
  const devName = devSummary?.device_name || deviceId

  let pointList = normalizeArrayResponse(await listDevicePointSources(channelId, deviceId))
  if (pointList.length > 0) return pointList

  const devData = normalizeArrayResponse(
    await request.get(`/api/channels/${encodeURIComponent(channelId)}/devices`, { silent: true })
  )
  const dev = devData.find(d => d.id === deviceId)
  if (dev?.points?.length) {
    return dev.points.map(pt => mapPointToSource(pt, channelId, channelName, deviceId, devName))
  }
  return []
}

async function refreshSourceValuesIfOnline(channelId, deviceId) {
  const devSummary = resolveDeviceSummary(channelId, deviceId)
  if (!isDeviceOnline(devSummary)) return
  const cacheKey = `${channelId}::${deviceId}`
  const points = devicePointsCache.get(cacheKey) || activeDevicePoints.value
  if (!points.length) return
  await loadSourceValueMap(points)
}

async function searchSourceDevices() {
  if (!sourceChannelId.value) {
    clearDeviceSearchResults()
    return
  }
  deviceSearchLoading.value = true
  deviceSearchDone.value = false
  try {
    const channelId = sourceChannelId.value
    const ch = channels.value.find(c => c.id === channelId)
    const channelName = ch?.name || channelId
    const q = deviceSearch.value.trim()

    // 优先走通道设备 API（与设备列表页一致，最可靠）
    let list = normalizeArrayResponse(
      await request.get(`/api/channels/${encodeURIComponent(channelId)}/devices`)
    ).map(dev => mapDeviceToSummary(dev, channelId, channelName))

    if (q) {
      list = list.filter(d =>
        fuzzyMatch(`${d.device_name} ${d.device_id} ${d.channel_name}`, q)
      )
    }

    // 若通道 API 无数据，再尝试虚拟影子检索 API
    if (list.length === 0) {
      const params = { channel_id: channelId, limit: 100 }
      if (q) params.q = q
      const res = await searchVirtualShadowDevices(params)
      list = normalizeArrayResponse(res).map(dev => ({
        key: `${channelId}::${dev.device_id || dev.id}`,
        channel_id: channelId,
        channel_name: channelName || dev.channel_name || channelId,
        device_id: dev.device_id || dev.id,
        device_name: dev.device_name || dev.name || dev.device_id || dev.id,
        point_count: dev.point_count ?? dev.pointCount ?? 0,
        online: dev.online ?? (dev.state === 0 || dev.state === 1),
        state: dev.state ?? 2
      }))
    }

    deviceSearchResults.value = list
  } catch (e) {
    deviceSearchResults.value = []
    console.error('[VirtualShadow] load devices failed', e)
    Message.error('加载设备列表失败')
  } finally {
    deviceSearchLoading.value = false
    deviceSearchDone.value = true
  }
}

function clearDeviceSearchResults() {
  deviceSearchResults.value = []
  deviceSearchDone.value = false
  devicePointsPrefetch.clear()
  devicePointsCache.clear()
}

function onSourceChannelChange() {
  deviceSearch.value = ''
  selectedSourceDevice.value = null
  activeDevicePoints.value = []
  selectedPointRefs.clear()
  if (sourceChannelId.value) {
    searchSourceDevices()
  } else {
    clearDeviceSearchResults()
  }
}

function onDeviceSearchClear() {
  deviceSearch.value = ''
  if (sourceChannelId.value) searchSourceDevices()
  else clearDeviceSearchResults()
}

async function loadDevicePoints(channelId, deviceId, { force = false } = {}) {
  const cacheKey = `${channelId}::${deviceId}`
  if (!force && devicePointsCache.has(cacheKey)) {
    activeDevicePoints.value = devicePointsCache.get(cacheKey)
    cacheSources(activeDevicePoints.value)
    refreshSourceValuesIfOnline(channelId, deviceId)
    return
  }

  pointsLoading.value = true
  try {
    const pointList = await fetchDevicePointSources(channelId, deviceId)
    devicePointsCache.set(cacheKey, pointList)
    activeDevicePoints.value = pointList
    cacheSources(pointList)
  } catch (e) {
    activeDevicePoints.value = []
    console.error('[VirtualShadow] load points failed', e)
    Message.error('加载设备点位失败')
  } finally {
    pointsLoading.value = false
  }
  refreshSourceValuesIfOnline(channelId, deviceId)
}

async function reloadDevicePoints() {
  if (!selectedSourceDevice.value) return
  const { channel_id, device_id } = selectedSourceDevice.value
  await loadDevicePoints(channel_id, device_id, { force: true })
}

async function fetchAll() {
  loading.value = true
  try {
    const [list, chList] = await Promise.all([
      listVirtualShadows(),
      request.get('/api/channels')
    ])
    devices.value = normalizeArrayResponse(list)
    channels.value = normalizeArrayResponse(chList)
    await Promise.all(devices.value.map(d => refreshRuntime(d.id, false)))
  } catch (_) {
    Message.error('加载虚拟影子设备失败')
  } finally {
    loading.value = false
  }
}

async function refreshAllRuntimes() {
  if (!devices.value.length) {
    await fetchAll()
    return
  }
  loading.value = true
  try {
    await Promise.all(devices.value.map(d => refreshRuntime(d.id, true)))
    Message.success('当前值已刷新')
  } catch (_) {
    Message.error('刷新当前值失败')
  } finally {
    loading.value = false
  }
}

async function refreshRuntime(id, recompute = false) {
  try {
    const res = await getVirtualShadow(id, { refresh: recompute })
    if (res?.runtime) {
      runtimeMap[id] = res.runtime
    } else {
      delete runtimeMap[id]
    }
  } catch (_) {
    delete runtimeMap[id]
  }
}

async function loadPreviewValues(id, recompute = true) {
  Object.keys(previewValues).forEach(k => delete previewValues[k])
  if (!id) return
  try {
    const res = await getVirtualShadow(id, { refresh: recompute })
    const pts = res?.runtime?.points || {}
    for (const [pid, info] of Object.entries(pts)) {
      previewValues[pid] = info
    }
  } catch (_) {
    /* ignore */
  }
}

function resetBuilderPicker() {
  deviceSearch.value = ''
  sourceChannelId.value = ''
  clearDeviceSearchResults()
  pointFilter.value = ''
  selectedSourceDevice.value = null
  activeDevicePoints.value = []
  pointsLoading.value = false
  selectedPointRefs.clear()
  batchDropActive.value = false
  pointListDragOver.value = false
  dropHoverIndex.value = -1
  mapDropHoverIndex.value = -1
}

function resetForm(record) {
  Object.assign(form, newVirtualDeviceForm())
  activePointIndex.value = 0
  duplicateLocateCursor.value = 0
  resetBuilderPicker()
  if (record) {
    editingId.value = record.id
    form.id = record.id
    form.name = record.name
    form.description = record.description || ''
    form.enable = record.enable !== false
    form.points = (record.points || []).map(p => ({ ...p }))
    const inferred = inferSourceChannelFromPoints()
    if (inferred) sourceChannelId.value = inferred
    loadPreviewValues(record.id)
  } else {
    editingId.value = ''
    if (channels.value.length === 1) {
      sourceChannelId.value = channels.value[0].id
    }
  }
}

async function openBuilder(record) {
  resetForm(record)
  builderVisible.value = true
  await nextTick()
  if (sourceChannelId.value) {
    await searchSourceDevices()
  }
}

function closeBuilder() {
  builderVisible.value = false
}

function addPoint(mode) {
  form.points.push(newVirtualPoint(mode))
  activePointIndex.value = form.points.length - 1
}

function removePoint(idx) {
  form.points.splice(idx, 1)
  if (activePointIndex.value >= form.points.length) {
    activePointIndex.value = Math.max(0, form.points.length - 1)
  }
}

function movePoint(idx, delta) {
  const next = idx + delta
  if (next < 0 || next >= form.points.length) return
  const item = form.points.splice(idx, 1)[0]
  form.points.splice(next, 0, item)
  activePointIndex.value = next
}

function selectSourceDevice(dev) {
  selectedSourceDevice.value = dev
  pointFilter.value = ''
  selectedPointRefs.clear()
  loadDevicePoints(dev.channel_id, dev.device_id)
}

function clearDevicePressHint() {
  if (devicePressTimer) {
    clearTimeout(devicePressTimer)
    devicePressTimer = null
  }
  devicePressHint.visible = false
  devicePressHint.key = ''
  devicePressState.key = ''
  devicePressState.pressing = false
}

function onDeviceCardPressStart(dev) {
  clearDevicePressHint()
  devicePressState.key = dev.key
  devicePressState.pressing = true
  devicePressHint.key = dev.key
  devicePressTimer = setTimeout(() => {
    devicePressHint.visible = true
    prefetchDevicePointsForDrag(dev)
  }, DEVICE_PRESS_HINT_MS)
}

function onDeviceCardPressEnd() {
  if (devicePressTimer) {
    clearTimeout(devicePressTimer)
    devicePressTimer = null
  }
  devicePressState.pressing = false
  if (!dragState.active) {
    devicePressHint.visible = false
    devicePressHint.key = ''
    devicePressState.key = ''
  }
}

function onDeviceCardClick(dev) {
  if (deviceDidDrag) {
    deviceDidDrag = false
    return
  }
  selectSourceDevice(dev)
}

async function prefetchDevicePointsForDrag(dev) {
  if (devicePointsPrefetch.has(dev.key)) return
  try {
    const pointList = await fetchDevicePointSources(dev.channel_id, dev.device_id)
    devicePointsPrefetch.set(dev.key, pointList.map(p => p.ref))
    cacheSources(pointList)
    devicePointsCache.set(`${dev.channel_id}::${dev.device_id}`, pointList)
  } catch (_) {
    /* prefetch is best-effort */
  }
}

function onDeviceDragStart(e, dev) {
  deviceDidDrag = true
  clearDevicePressHint()
  draggingDeviceKey.value = dev.key
  const refs = devicePointsPrefetch.get(dev.key) || []
  const label = `${dev.device_name} · ${dev.point_count} 点`
  setDragPayload(e, refs, label, {
    device: {
      key: dev.key,
      channel_id: dev.channel_id,
      device_id: dev.device_id,
      device_name: dev.device_name,
      point_count: dev.point_count
    }
  })
}

async function handleDeviceDrop(device) {
  const devSummary =
    deviceSearchResults.value.find(d => d.key === device.key) ||
    deviceSearchResults.value.find(d => d.device_id === device.device_id) ||
    device
  if (
    !selectedSourceDevice.value ||
    selectedSourceDevice.value.device_id !== device.device_id
  ) {
    selectedSourceDevice.value = devSummary
    pointFilter.value = ''
    selectedPointRefs.clear()
    await loadDevicePoints(device.channel_id, device.device_id)
  } else if (!activeDevicePoints.value.length) {
    await loadDevicePoints(device.channel_id, device.device_id)
  }
  const refs = activeDevicePoints.value.map(p => p.ref)
  const added = addMapBlocksFromRefs(refs)
  if (added > 0) {
    Message.success(`已从 ${devSummary.device_name} 批量创建 ${added} 个映射积木`)
  } else {
    Message.info('该设备点位均已映射，未重复添加')
  }
}

function clearSourceDevice() {
  selectedSourceDevice.value = null
  activeDevicePoints.value = []
  pointFilter.value = ''
  selectedPointRefs.clear()
}

function togglePointSelection(ref) {
  if (suppressClick) return
  if (selectedPointRefs.has(ref)) selectedPointRefs.delete(ref)
  else selectedPointRefs.add(ref)
}

function toggleSelectAllDevicePoints(checked) {
  selectedPointRefs.clear()
  if (checked) {
    for (const p of filteredDevicePoints.value) selectedPointRefs.add(p.ref)
  }
}

function existingMapRefs() {
  return new Set(
    form.points.filter(p => p.mode === 'map' && p.source_ref).map(p => p.source_ref)
  )
}

function addMapBlockFromSource(src, skipDuplicate = true) {
  if (!src) return false
  if (skipDuplicate && existingMapRefs().has(src.ref)) return false
  const pt = newVirtualPoint('map')
  pt.source_ref = src.ref
  pt.point_id = src.point_id
  pt.name = src.point_name || src.point_id
  form.points.push(pt)
  activePointIndex.value = form.points.length - 1
  return true
}

function addMapBlocksFromRefs(refs) {
  let added = 0
  for (const ref of refs) {
    const src = resolveSource(ref)
    if (addMapBlockFromSource(src, true)) added++
  }
  if (added > 0 && !form.name && selectedSourceDevice.value) {
    form.name = `${selectedSourceDevice.value.device_name}-虚拟`
  }
  if (added > 0) warnDuplicatePointIdsIfAny()
  return added
}

function batchAddSelectedToMapping() {
  const added = addMapBlocksFromRefs([...selectedPointRefs])
  if (added > 0) {
    Message.success(`已添加 ${added} 个映射点位`)
  } else {
    Message.info('所选点位均已存在映射')
  }
}

function refsForDrag(src) {
  if (selectedPointRefs.has(src.ref) && selectedPointRefs.size > 0) {
    return [...selectedPointRefs]
  }
  return [src.ref]
}

function setDragPayload(e, refs, label, { device = null } = {}) {
  if (!e.dataTransfer) return
  e.stopPropagation()
  e.dataTransfer.effectAllowed = 'copy'
  const encoded = encodeDragPayload({ refs, device })
  e.dataTransfer.setData(DRAG_MIME, encoded)
  e.dataTransfer.setData('text/plain', encoded)

  dragState.active = true
  dragState.count = refs.length || device?.point_count || 1
  dragState.label = label || refs[0] || device?.device_name || '点位'
  draggingRefs.clear()
  for (const r of refs) draggingRefs.add(r)

  if (dragGhostEl) {
    dragGhostEl.remove()
    dragGhostEl = null
  }
  const ghost = document.createElement('div')
  ghost.className = 'vs-drag-ghost' + (device ? ' vs-drag-ghost--device' : '')
  const icon = document.createElement('span')
  icon.className = 'vs-drag-ghost-icon'
  icon.textContent = '⋮⋮'
  const text = document.createElement('span')
  text.className = 'vs-drag-ghost-text'
  text.textContent =
    device && refs.length > 1
      ? `${device.device_name} · ${refs.length} 点`
      : device
        ? `${device.device_name} · ${device.point_count || refs.length || 0} 点`
        : refs.length > 1
          ? `${refs.length} 个点位`
          : (label || refs[0] || '')
  ghost.appendChild(icon)
  ghost.appendChild(text)
  Object.assign(ghost.style, {
    position: 'fixed',
    top: '-1000px',
    left: '-1000px',
    pointerEvents: 'none',
    zIndex: '99999'
  })
  document.body.appendChild(ghost)
  dragGhostEl = ghost
  e.dataTransfer.setDragImage(ghost, 28, 22)
}

function onPointDragStart(e, src) {
  const refs = refsForDrag(src)
  const label = src.point_name || src.point_id
  setDragPayload(e, refs, label)
}

function onBatchDragStart(e) {
  if (!selectedPointRefs.size) return
  const refs = [...selectedPointRefs]
  setDragPayload(e, refs, `${refs.length} 个点位`)
}

function clearDragVisualState() {
  dragState.active = false
  dragState.count = 0
  dragState.label = ''
  draggingRefs.clear()
  draggingDeviceKey.value = null
  batchDropActive.value = false
  pointListDragOver.value = false
  dropHoverIndex.value = -1
  mapDropHoverIndex.value = -1
  clearDevicePressHint()
}

function onDragEnd() {
  if (dragGhostEl) {
    dragGhostEl.remove()
    dragGhostEl = null
  }
  clearDragVisualState()
}

function onBatchZoneDragOver(e) {
  e.preventDefault()
  if (e.dataTransfer) e.dataTransfer.dropEffect = 'copy'
  batchDropActive.value = true
}

function onBatchZoneDragLeave(e) {
  if (e.currentTarget?.contains(e.relatedTarget)) return
  batchDropActive.value = false
}

async function applyBatchDropPayload(payload) {
  if (payload.device) {
    if (payload.refs.length) {
      const added = addMapBlocksFromRefs(payload.refs)
      if (added > 0) {
        Message.success(`已从 ${payload.device.device_name} 批量创建 ${added} 个映射积木`)
      } else {
        Message.info('该设备点位均已映射，未重复添加')
      }
      return
    }
    await handleDeviceDrop(payload.device)
    return
  }
  const refs = payload.refs
  if (!refs.length) return
  const added = addMapBlocksFromRefs(refs)
  if (added > 0) Message.success(`批量创建了 ${added} 个映射积木`)
  else Message.info('点位均已映射，未重复添加')
}

function applyMapDropPayload(payload, idx) {
  activePointIndex.value = idx
  mapDropHoverIndex.value = -1
  dropHoverIndex.value = -1
  const refs = payload.refs
  if (!refs.length) return
  const pt = form.points[idx]
  if (!pt) return
  if (pt.mode === 'map') {
    const src = resolveSource(refs[0])
    if (src) applyRefToPoint(idx, src)
  } else if (refs.length === 1) {
    insertAtFormula(idx, refs[0])
  } else {
    insertAtFormula(idx, refs.join(' + '))
  }
}

function applyBlockDropPayload(payload, idx) {
  activePointIndex.value = idx
  dropHoverIndex.value = -1
  const refs = payload.refs
  if (!refs.length) return
  const pt = form.points[idx]
  if (!pt) return
  if (pt.mode === 'map') {
    const src = resolveSource(refs[0])
    if (src) applyRefToPoint(idx, src)
  } else if (refs.length === 1) {
    insertAtFormula(idx, refs[0])
  } else {
    insertAtFormula(idx, refs.join(' + '))
  }
}

async function onBatchZoneDrop(e) {
  const payload = decodeDragPayload(e.dataTransfer)
  clearDragVisualState()
  await applyBatchDropPayload(payload)
}

function onMapZoneDragOver(idx, e) {
  e?.preventDefault?.()
  if (e?.dataTransfer) e.dataTransfer.dropEffect = 'copy'
  mapDropHoverIndex.value = idx
  dropHoverIndex.value = idx
}

function onMapZoneDragLeave() {
  mapDropHoverIndex.value = -1
}

function onMapZoneDrop(e, idx) {
  const payload = decodeDragPayload(e.dataTransfer)
  clearDragVisualState()
  applyMapDropPayload(payload, idx)
}

function applyRefToPoint(idx, src) {
  const pt = form.points[idx]
  if (!pt || !src) return
  if (pt.mode === 'map') {
    pt.source_ref = src.ref
    if (!pt.point_id) pt.point_id = src.point_id
    if (!pt.name) pt.name = src.point_name || src.point_id
    if (isDuplicatePointId(idx)) warnDuplicatePointIdsIfAny()
  } else {
    insertAtFormula(idx, src.ref)
  }
}

function onBlockDragOver(idx, e) {
  e?.preventDefault?.()
  if (e?.dataTransfer) e.dataTransfer.dropEffect = 'copy'
  dropHoverIndex.value = idx
}

function onBlockDragLeave() {
  dropHoverIndex.value = -1
}

function onBlockDrop(e, idx) {
  const payload = decodeDragPayload(e.dataTransfer)
  clearDragVisualState()
  applyBlockDropPayload(payload, idx)
}

function insertFormula(text) {
  insertAtFormula(activePointIndex.value, text)
}

function insertAtFormula(idx, text) {
  const pt = form.points[idx]
  if (!pt || pt.mode !== 'formula') return
  const cur = pt.formula || ''
  const needSpace = cur && !cur.endsWith(' ') && text !== ')' && text !== '('
  pt.formula = cur + (needSpace ? ' ' : '') + text
}

function applyFormulaTemplate(key) {
  addPoint('formula')
  const idx = form.points.length - 1
  const pt = form.points[idx]
  pt.point_id = pt.point_id || `calc_${key}`
  const templates = {
    sum: 'ref_a + ref_b',
    diff: 'ref_a - ref_b',
    avg: '(ref_a + ref_b) / 2',
    scale: 'ref_a * 1.5'
  }
  pt.formula = templates[key] || ''
  activePointIndex.value = idx
  Message.info('请将 ref_a / ref_b 替换为左侧点位引用')
}

async function saveDevice() {
  const payload = {
    id: form.id.trim(),
    name: form.name.trim(),
    description: form.description.trim(),
    enable: form.enable,
    points: form.points.map(p => ({
      point_id: p.point_id?.trim(),
      name: p.name?.trim(),
      unit: p.unit?.trim(),
      mode: p.mode,
      source_ref: p.source_ref?.trim(),
      formula: p.formula?.trim()
    }))
  }
  if (!payload.id) {
    Message.warning('请填写设备 ID')
    return false
  }
  if (idError.value) {
    Message.warning(idError.value)
    return false
  }
  if (!payload.points.length) {
    Message.warning('请至少添加一个虚拟点位')
    return false
  }
  if (duplicatePointIdMap.value.size > 0) {
    Message.warning(`虚拟点位 ID 不得重复：${duplicatePointIdSummary()}`)
    duplicateLocateCursor.value = 0
    focusNextDuplicatePointBlock({ announce: false })
    return false
  }
  for (const p of payload.points) {
    if (!p.point_id) {
      Message.warning('每个积木需填写虚拟点位 ID')
      return false
    }
    if (p.mode === 'map' && !p.source_ref) {
      Message.warning(`点位 ${p.point_id} 需选择映射来源`)
      return false
    }
    if (p.mode === 'formula' && !p.formula) {
      Message.warning(`点位 ${p.point_id} 需填写计算公式`)
      return false
    }
  }
  try {
    if (editingId.value) {
      await updateVirtualShadow(editingId.value, payload)
      Message.success('已更新')
    } else {
      await createVirtualShadow(payload)
      Message.success('已创建')
    }
    await fetchAll()
    await refreshRuntime(payload.id, true)
    return true
  } catch (e) {
    const status = e?.response?.status
    let msg = e?.response?.data?.error || e?.message || '保存失败'
    if (status === 405) {
      msg = '保存接口不可用 (405)，请重新编译并重启后端服务后再试'
    } else if (status === 503) {
      msg = '虚拟影子服务未就绪，请确认网关已完整启动'
    }
    Message.error(msg)
    return false
  }
}

async function removeDevice(id) {
  try {
    await deleteVirtualShadow(id)
    Message.success('已删除')
    await fetchAll()
  } catch (_) {
    Message.error('删除失败')
  }
}

function openDetail(record) {
  detailDevice.value = record
  detailRuntime.value = runtimeMap[record.id] || null
  detailVisible.value = true
  refreshDetailRuntime()
}

async function refreshDetailRuntime() {
  if (!detailDevice.value) return
  detailLoading.value = true
  try {
    const res = await getVirtualShadow(detailDevice.value.id, { refresh: true })
    detailRuntime.value = res?.runtime || null
    if (res?.runtime) {
      runtimeMap[detailDevice.value.id] = res.runtime
    } else {
      Message.warning('暂无运行时数据，请确认源设备有点位采集且虚拟设备已启用')
    }
  } catch (_) {
    detailRuntime.value = null
    Message.error('刷新实时值失败')
  } finally {
    detailLoading.value = false
  }
}

const virtualDeviceIds = computed(() => new Set(devices.value.map(d => d.id)))

function applyWsPointUpdate(channelId, deviceId, pointId, payload) {
  const info = {
    value: payload.value,
    quality: payload.quality,
    timestamp: payload.collected_at || payload.timestamp,
    collected_at: payload.collected_at,
    updated_at: payload.updated_at
  }

  const ref = makePointRef(channelId, deviceId, pointId)
  sourceValueMap[ref] = info

  if (virtualDeviceIds.value.has(deviceId)) {
    if (!runtimeMap[deviceId]) {
      runtimeMap[deviceId] = { points: {} }
    }
    if (!runtimeMap[deviceId].points) {
      runtimeMap[deviceId].points = {}
    }
    runtimeMap[deviceId].points[pointId] = info
    if (detailDevice.value?.id === deviceId && detailRuntime.value) {
      if (!detailRuntime.value.points) detailRuntime.value.points = {}
      detailRuntime.value.points[pointId] = info
    }
    if (editingId.value === deviceId) {
      previewValues[pointId] = info
    }
  }
}

let ws = null
function connectWs() {
  if (ws) return
  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
  ws = new WebSocket(`${protocol}//${window.location.host}/api/ws/values`)
  ws.onmessage = (event) => {
    try {
      const data = JSON.parse(event.data)
      if (!data?.channel_id || !data?.device_id || !data?.point_id) return
      applyWsPointUpdate(data.channel_id, data.device_id, data.point_id, data)
    } catch (_) {
      /* ignore */
    }
  }
  ws.onclose = () => {
    ws = null
    setTimeout(connectWs, 3000)
  }
}

async function applyPrefillFromQuery() {
  if (route.query.new !== '1') return
  const channelId = String(route.query.channel_id || '')
  const refsRaw = String(route.query.refs || '')
  const refs = refsRaw.split(',').map(s => s.trim()).filter(Boolean)
  if (!refs.length) return

  resetForm(null)
  if (channelId) {
    sourceChannelId.value = channelId
  }
  addMapBlocksFromRefs(refs)

  const first = parsePointRef(refs[0])
  if (first) {
    for (const ref of refs) {
      selectedPointRefs.add(ref)
      cacheSources([resolveSource(ref)])
    }
    deviceSearch.value = first.deviceId
    await searchSourceDevices()
    const dev = deviceSearchResults.value.find(d => d.key === `${first.channelId}::${first.deviceId}`)
    if (dev) {
      selectSourceDevice(dev)
      if (!form.name) form.name = `${dev.device_name}-虚拟`
    } else {
      selectedSourceDevice.value = {
        key: `${first.channelId}::${first.deviceId}`,
        channel_id: first.channelId,
        device_id: first.deviceId,
        device_name: first.deviceId,
        channel_name: first.channelId,
        point_count: refs.length
      }
      await loadDevicePoints(first.channelId, first.deviceId)
    }
  }
  if (!form.id) form.id = `virtual-${Date.now().toString(36)}`
  builderVisible.value = true
  router.replace({ path: '/virtual-shadows' })
}

onMounted(async () => {
  dragUsesPointer.value = isAppleOrTouchDrag()
  isMacPlatform.value = isMac()
  if (isMacPlatform.value) {
    try {
      macHintVisible.value = !localStorage.getItem(MAC_HINT_STORAGE_KEY)
    } catch (_) {
      macHintVisible.value = true
    }
  }
  await fetchAll()
  await applyPrefillFromQuery()
  connectWs()
})

onBeforeUnmount(() => {
  clearMacHintFadeTimer()
  clearDevicePressHint()
  endPointerDragSession()
  if (dragGhostEl) {
    dragGhostEl.remove()
    dragGhostEl = null
  }
  if (ws) {
    ws.close()
    ws = null
  }
})
</script>
