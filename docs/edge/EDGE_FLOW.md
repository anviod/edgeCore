---
layout: default
title: 边缘计算SVG流程图绘制指南
description: edgeCore 边缘计算SVG流程图绘制指南
---

# 边缘计算SVG流程图绘制指南
<svg width="1200" height="1000" viewBox="0 0 1200 1000" xmlns="http://www.w3.org/2000/svg">
  <defs>
    <marker id="arrow" markerWidth="8" markerHeight="8" refX="7" refY="4" orient="auto">
      <path d="M0,0 L8,4 L0,8 Z" fill="#94a3b8"/>
    </marker>
    <marker id="arrow-active" markerWidth="8" markerHeight="8" refX="7" refY="4" orient="auto">
      <path d="M0,0 L8,4 L0,8 Z" fill="#6c5ce7"/>
    </marker>
  </defs>

  <rect width="1200" height="1000" fill="#fcfcfd" />
  
  <g transform="translate(50, 60)">
    <text x="550" y="-20" text-anchor="middle" font-family="PingFang SC, monospace" font-size="20" font-weight="bold" fill="#1e293b">EDGE_OS 边缘计算集成架构 (V2.4.0)</text>
    
    <rect x="0" y="20" width="280" height="150" fill="none" stroke="#0ea5e9" stroke-width="1.5" stroke-dasharray="4,2"/>
    <text x="10" y="15" font-family="PingFang SC, monospace" font-size="12" fill="#0ea5e9" font-weight="bold">01 数据采集层 (Ingestion)</text>
    <g transform="translate(20, 50)">
      <rect width="110" height="35" fill="#fff" stroke="#cbd5e1" />
      <text x="55" y="22" text-anchor="middle" font-family="monospace" font-size="10">Modbus/S7</text>
      <rect x="120" width="110" height="35" fill="#fff" stroke="#cbd5e1" />
      <text x="175" y="22" text-anchor="middle" font-family="monospace" font-size="10">OPC-UA</text>
      <rect y="45" width="230" height="35" fill="#fff" stroke="#cbd5e1" />
      <text x="115" y="67" text-anchor="middle" font-family="monospace" font-size="10">MQTT / HTTP API</text>
    </g>

    <rect x="400" y="20" width="350" height="150" fill="#f5f3ff" stroke="#6c5ce7" stroke-width="1.5"/>
    <text x="410" y="15" font-family="PingFang SC, monospace" font-size="12" fill="#6c5ce7" font-weight="bold">02 核心引擎层 (Processing)</text>
    <g transform="translate(420, 50)">
      <rect width="140" height="80" fill="#fff" stroke="#6c5ce7" stroke-dasharray="2,2" />
      <text x="70" y="40" text-anchor="middle" font-family="PingFang SC, monospace" font-size="11">影子设备</text>
      <text x="70" y="55" text-anchor="middle" font-family="monospace" font-size="9" fill="#94a3b8">Virtual Shadow Engine</text>
      
      <rect x="170" width="140" height="80" fill="#fff" stroke="#6c5ce7" stroke-dasharray="2,2" />
      <text x="240" y="40" text-anchor="middle" font-family="PingFang SC, monospace" font-size="11">规则解析器</text>
      <text x="240" y="55" text-anchor="middle" font-family="monospace" font-size="9" fill="#94a3b8">Rule Engine</text>
    </g>

    <rect x="850" y="20" width="250" height="150" fill="none" stroke="#334155" stroke-width="1.5"/>
    <text x="860" y="15" font-family="PingFang SC, monospace" font-size="12" fill="#334155" font-weight="bold">03 导出与同步 (Export)</text>
    <g transform="translate(870, 50)">
      <rect width="210" height="35" fill="#fff" stroke="#cbd5e1" />
      <text x="105" y="22" text-anchor="middle" font-family="PingFang SC, monospace" font-size="10">设备画像 (Device Profile)</text>
      <rect y="45" width="210" height="35" fill="#fff" stroke="#cbd5e1" />
      <text x="105" y="67" text-anchor="middle" font-family="PingFang SC, monospace" font-size="10">北向联动控制台同步</text>
    </g>

    <path d="M280 95 L400 95" stroke="#6c5ce7" stroke-width="1.5" marker-end="url(#arrow-active)" />
    <path d="M750 95 L850 95" stroke="#6c5ce7" stroke-width="1.5" marker-end="url(#arrow-active)" />
  </g>

  <g transform="translate(50, 280)">
    <rect width="1100" height="320" fill="#fff" stroke="#e2e8f0" stroke-width="1" />
    <text x="20" y="25" font-family="PingFang SC, monospace" font-size="14" font-weight="bold" fill="#1e293b">规则逻辑矩阵 (Logic Matrix)</text>
    
    <g transform="translate(30, 60)">
      <rect width="240" height="230" fill="#f8fafc" stroke="#6c5ce7" stroke-width="1"/>
      <text x="120" y="25" text-anchor="middle" font-family="PingFang SC, monospace" font-size="12" font-weight="bold">阈值判定 (Threshold)</text>
      <rect x="20" y="50" width="200" height="40" fill="#fff" stroke="#e2e8f0"/>
      <text x="30" y="75" font-family="monospace" font-size="10">IF x &gt; High_Bound</text>
      <rect x="20" y="100" width="200" height="40" fill="#fff" stroke="#e2e8f0"/>
      <text x="30" y="125" font-family="monospace" font-size="10">IF x &lt; Low_Bound</text>
      <path d="M220 70 L300 130" stroke="#cbd5e1" stroke-width="1" fill="none" marker-end="url(#arrow)"/>
    </g>

    <g transform="translate(320, 60)">
      <rect width="240" height="230" fill="#f8fafc" stroke="#6c5ce7" stroke-width="1"/>
      <text x="120" y="25" text-anchor="middle" font-family="PingFang SC, monospace" font-size="12" font-weight="bold">窗口算法 (Window)</text>
      <circle cx="120" cy="110" r="40" fill="none" stroke="#6c5ce7" stroke-dasharray="4,2" />
      <text x="120" y="115" text-anchor="middle" font-family="PingFang SC, monospace" font-size="10">滑动平均 / 累计</text>
      <path d="M220 130 L300 130" stroke="#cbd5e1" stroke-width="1" fill="none" marker-end="url(#arrow)"/>
    </g>

    <path d="M560 175 L620 175" stroke="#6c5ce7" stroke-width="2" marker-end="url(#arrow-active)" />

    <g transform="translate(620, 60)">
      <rect width="450" height="230" fill="#1e293b" rx="2"/>
      <text x="225" y="25" text-anchor="middle" font-family="PingFang SC, monospace" font-size="12" font-weight="bold" fill="#fff">动作执行器 (Executors)</text>
      <g transform="translate(30, 50)">
        <text x="0" y="20" font-family="monospace" font-size="11" fill="#0ea5e9">[指令] 设备反控 (Write_Device)</text>
        <text x="0" y="50" font-family="monospace" font-size="11" fill="#22c55e">[消息] MQTT 推送 (Publish)</text>
        <text x="0" y="80" font-family="monospace" font-size="11" fill="#eab308">[告警] 钉钉/邮件 (Webhook)</text>
        <text x="0" y="110" font-family="monospace" font-size="11" fill="#94a3b8">[存储] 本地持久化 (SQLite)</text>
      </g>
    </g>
  </g>

  <g transform="translate(50, 640)">
    <text x="0" y="-15" font-family="PingFang SC, monospace" font-size="14" font-weight="bold" fill="#1e293b">典型应用场景 (Use Cases)</text>
    
    <rect x="0" y="0" width="340" height="180" fill="#fff" stroke="#e2e8f0"/>
    <text x="20" y="30" font-family="PingFang SC, monospace" font-size="12" font-weight="bold">智能工厂：CNC 预测性维护</text>
    <text x="20" y="60" font-family="PingFang SC, monospace" font-size="10" fill="#64748b">监测：主轴振动频率 (FFT)</text>
    <text x="20" y="85" font-family="PingFang SC, monospace" font-size="10" fill="#64748b">逻辑：偏离基准值 &gt; 15%</text>
    <text x="20" y="110" font-family="PingFang SC, monospace" font-size="10" fill="#6c5ce7">执行：自动停机并推送维保工单</text>

    <rect x="380" y="0" width="340" height="180" fill="#fff" stroke="#e2e8f0"/>
    <text x="400" y="30" font-family="PingFang SC, monospace" font-size="12" font-weight="bold">智慧能源：配电房负荷平衡</text>
    <text x="400" y="60" font-family="PingFang SC, monospace" font-size="10" fill="#64748b">监测：三相电流 / 有功功率</text>
    <text x="400" y="85" font-family="PingFang SC, monospace" font-size="10" fill="#64748b">逻辑：峰谷电价阶梯计算</text>
    <text x="400" y="110" font-family="PingFang SC, monospace" font-size="10" fill="#6c5ce7">执行：动态切断三级非必要负荷</text>

    <rect x="760" y="0" width="340" height="180" fill="#fff" stroke="#e2e8f0"/>
    <text x="780" y="30" font-family="PingFang SC, monospace" font-size="12" font-weight="bold">智慧楼宇：空调节能闭环</text>
    <text x="780" y="60" font-family="PingFang SC, monospace" font-size="10" fill="#64748b">监测：人流量 + 二氧化碳浓度</text>
    <text x="780" y="85" font-family="PingFang SC, monospace" font-size="10" fill="#64748b">逻辑：CO2 &lt; 400ppm 且无人</text>
    <text x="780" y="110" font-family="PingFang SC, monospace" font-size="10" fill="#6c5ce7">执行：调低新风频率 30%</text>
  </g>

  <path d="M1100 170 L1160 170 L1160 920 L50 920 L50 210" stroke="#94a3b8" stroke-width="1.5" stroke-dasharray="8,4" fill="none" marker-end="url(#arrow)"/>
  <text x="600" y="910" text-anchor="middle" font-family="PingFang SC, monospace" font-size="11" fill="#94a3b8">反控闭环：从执行结果到采集策略的自动优化 (Closed-loop Optimization)</text>
</svg>


设计亮点说明：

色彩一致性：

亮蓝 (#0ea5e9)：用于“数据输入层”、“边缘设备层”及三个实战案例的输入外框。

紫色 (#6c5ce7)：用于“规则匹配”、“规则执行”及“核心引擎层”的填充与文字。

边框与字体：

全部使用 1px - 1.5px 精细线条，无圆角（或极小圆角 rx="2"），符合工业硬朗美学。

字体统一使用 monospace，增强代码感。

粒子动效同步：

定义了全局同步的 animateMotion。顶层架构使用 1.5s 快速流动展示高频采集，底部矩阵与案例展示使用 2s 稳定流动展示逻辑处理。

全要素覆盖：

图中垂直展示了从原子级规则（阈值/窗口等）到原子级动作（MQTT/HTTP等），再到系统级分层，最后落地到业务场景案例的完整逻辑。

反馈闭环：

增加了一个覆盖全图的虚线反馈环（Feedback Loop），体现了边缘计算从执行到优化的闭环特性。

更新说明
中文语义标注：

将原有的英文图表层级翻译为符合工业语境的中文（如：数据采集层、规则解析器、预测性维护等）。

保留了关键的英文术语（括号内），方便开发时与 API 字段对应。

关联线条完善：

主流程线：增加了带颜色的实线，清晰标注了数据从采集到导出、分发的路径。

逻辑汇聚线：在规则矩阵中，增加了从“阈值判定”和“窗口算法”指向“动作执行器”的引导线。

反馈闭环线：使用了大跨度的虚线，体现了 EDGE_OS 的核心价值——闭环反控（即根据分析结果自动去调节底层设备）, 使用又出左进 首尾都在横线上。

视觉分层：

使用深色背景块标识“动作执行器”，暗示其为高优先级的“暗盒操作”。

每个模块都带有了编号（01, 02, 03），强调了数据流动的时序性。

## 1. 核心工作流程SVG绘制步骤



### 流程概述

核心工作流程包括数据输入、规则匹配、规则执行、动作执行四个主要阶段。



### 绘制步骤

1. **创建SVG画布**

   ```svg

   <svg width="800" height="300" xmlns="http://www.w3.org/2000/svg">

   ```



2. **绘制四个主要节点**

   ```svg

   <!-- 数据输入层 -->

   <rect x="50" y="50" width="150" height="60" rx="5" fill="#f0f0f0" stroke="#333" stroke-width="1"/>

   <text x="125" y="80" text-anchor="middle" font-family="Arial" font-size="14">数据输入层</text>



   <!-- 规则匹配层 -->

   <rect x="250" y="50" width="150" height="60" rx="5" fill="#f0f0f0" stroke="#333" stroke-width="1"/>

   <text x="325" y="80" text-anchor="middle" font-family="Arial" font-size="14">规则匹配层</text>



   <!-- 规则执行层 -->

   <rect x="450" y="50" width="150" height="60" rx="5" fill="#f0f0f0" stroke="#333" stroke-width="1"/>

   <text x="525" y="80" text-anchor="middle" font-family="Arial" font-size="14">规则执行层</text>



   <!-- 动作执行层 -->

   <rect x="450" y="150" width="150" height="60" rx="5" fill="#f0f0f0" stroke="#333" stroke-width="1"/>

   <text x="525" y="180" text-anchor="middle" font-family="Arial" font-size="14">动作执行层</text>

   ```



3. **绘制连接线**

   ```svg

   <!-- 数据输入到规则匹配 -->

   <path d="M200 80 L250 80" stroke="#333" stroke-width="2" marker-end="url(#arrowhead)"/>



   <!-- 规则匹配到规则执行 -->

   <path d="M400 80 L450 80" stroke="#333" stroke-width="2" marker-end="url(#arrowhead)"/>



   <!-- 规则执行到动作执行 -->

   <path d="M525 110 L525 150" stroke="#333" stroke-width="2" marker-end="url(#arrowhead)"/>

   ```



4. **添加箭头标记**

   ```svg

   <defs>

     <marker id="arrowhead" markerWidth="10" markerHeight="7" refX="9" refY="3.5" orient="auto">

       <polygon points="0 0, 10 3.5, 0 7" fill="#333"/>

     </marker>

   </defs>

   ```



5. **关闭SVG标签**

   ```svg

   </svg>

   ```



## 2. 规则类型处理流程SVG绘制步骤



### 流程概述

包括阈值规则、计算规则、窗口规则和状态规则四种类型的处理流程。



### 绘制步骤

1. **创建SVG画布**

   ```svg

   <svg width="1000" height="400" xmlns="http://www.w3.org/2000/svg">

   ```



2. **绘制标题**

   ```svg

   <text x="500" y="30" text-anchor="middle" font-family="Arial" font-size="18" font-weight="bold">规则类型处理流程</text>

   ```



3. **绘制阈值规则流程**

   ```svg

   <!-- 阈值规则 -->

   <rect x="50" y="60" width="120" height="40" rx="5" fill="#e6f7ff" stroke="#1890ff" stroke-width="1"/>

   <text x="110" y="85" text-anchor="middle" font-family="Arial" font-size="14">条件评估</text>



   <rect x="200" y="60" width="120" height="40" rx="5" fill="#e6f7ff" stroke="#1890ff" stroke-width="1"/>

   <text x="260" y="85" text-anchor="middle" font-family="Arial" font-size="14">状态检查</text>



   <rect x="350" y="60" width="120" height="40" rx="5" fill="#e6f7ff" stroke="#1890ff" stroke-width="1"/>

   <text x="410" y="85" text-anchor="middle" font-family="Arial" font-size="14">动作触发</text>



   <path d="M170 80 L200 80" stroke="#1890ff" stroke-width="2" marker-end="url(#arrowhead)"/>

   <path d="M320 80 L350 80" stroke="#1890ff" stroke-width="2" marker-end="url(#arrowhead)"/>

   ```



4. **绘制计算规则流程**

   ```svg

   <!-- 计算规则 -->

   <rect x="50" y="140" width="120" height="40" rx="5" fill="#f6ffed" stroke="#52c41a" stroke-width="1"/>

   <text x="110" y="165" text-anchor="middle" font-family="Arial" font-size="14">表达式计算</text>



   <rect x="200" y="140" width="120" height="40" rx="5" fill="#f6ffed" stroke="#52c41a" stroke-width="1"/>

   <text x="260" y="165" text-anchor="middle" font-family="Arial" font-size="14">结果生成</text>



   <rect x="350" y="140" width="120" height="40" rx="5" fill="#f6ffed" stroke="#52c41a" stroke-width="1"/>

   <text x="410" y="165" text-anchor="middle" font-family="Arial" font-size="14">动作触发</text>



   <path d="M170 160 L200 160" stroke="#52c41a" stroke-width="2" marker-end="url(#arrowhead)"/>

   <path d="M320 160 L350 160" stroke="#52c41a" stroke-width="2" marker-end="url(#arrowhead)"/>

   ```



5. **绘制窗口规则流程**

   ```svg

   <!-- 窗口规则 -->

   <rect x="550" y="60" width="120" height="40" rx="5" fill="#fff7e6" stroke="#fa8c16" stroke-width="1"/>

   <text x="610" y="85" text-anchor="middle" font-family="Arial" font-size="14">数据收集</text>



   <rect x="700" y="60" width="120" height="40" rx="5" fill="#fff7e6" stroke="#fa8c16" stroke-width="1"/>

   <text x="760" y="85" text-anchor="middle" font-family="Arial" font-size="14">聚合计算</text>



   <rect x="850" y="60" width="120" height="40" rx="5" fill="#fff7e6" stroke="#fa8c16" stroke-width="1"/>

   <text x="910" y="85" text-anchor="middle" font-family="Arial" font-size="14">条件评估</text>



   <rect x="700" y="140" width="120" height="40" rx="5" fill="#fff7e6" stroke="#fa8c16" stroke-width="1"/>

   <text x="760" y="165" text-anchor="middle" font-family="Arial" font-size="14">结果生成</text>



   <path d="M670 80 L700 80" stroke="#fa8c16" stroke-width="2" marker-end="url(#arrowhead)"/>

   <path d="M820 80 L850 80" stroke="#fa8c16" stroke-width="2" marker-end="url(#arrowhead)"/>

   <path d="M910 100 L910 140" stroke="#fa8c16" stroke-width="2" marker-end="url(#arrowhead)"/>

   <path d="M850 160 L700 160" stroke="#fa8c16" stroke-width="2" marker-end="url(#arrowhead)"/>

   ```



6. **绘制状态规则流程**

   ```svg

   <!-- 状态规则 -->

   <rect x="550" y="220" width="120" height="40" rx="5" fill="#fff1f0" stroke="#f5222d" stroke-width="1"/>

   <text x="610" y="245" text-anchor="middle" font-family="Arial" font-size="14">条件评估</text>



   <rect x="700" y="220" width="120" height="40" rx="5" fill="#fff1f0" stroke="#f5222d" stroke-width="1"/>

   <text x="760" y="245" text-anchor="middle" font-family="Arial" font-size="14">状态计时</text>



   <rect x="850" y="220" width="120" height="40" rx="5" fill="#fff1f0" stroke="#f5222d" stroke-width="1"/>

   <text x="910" y="245" text-anchor="middle" font-family="Arial" font-size="14">计数检查</text>



   <rect x="700" y="300" width="120" height="40" rx="5" fill="#fff1f0" stroke="#f5222d" stroke-width="1"/>

   <text x="760" y="325" text-anchor="middle" font-family="Arial" font-size="14">动作触发</text>



   <path d="M670 240 L700 240" stroke="#f5222d" stroke-width="2" marker-end="url(#arrowhead)"/>

   <path d="M820 240 L850 240" stroke="#f5222d" stroke-width="2" marker-end="url(#arrowhead)"/>

   <path d="M910 260 L910 300" stroke="#f5222d" stroke-width="2" marker-end="url(#arrowhead)"/>

   <path d="M850 320 L700 320" stroke="#f5222d" stroke-width="2" marker-end="url(#arrowhead)"/>

   ```



7. **添加箭头标记**

   ```svg

   <defs>

     <marker id="arrowhead" markerWidth="10" markerHeight="7" refX="9" refY="3.5" orient="auto">

       <polygon points="0 0, 10 3.5, 0 7" fill="#333"/>

     </marker>

   </defs>

   ```



8. **关闭SVG标签**

   ```svg

   </svg>

   ```



## 3. 动作类型执行流程SVG绘制步骤



### 流程概述

包括设备控制、MQTT发布、HTTP请求和数据库存储四种动作类型的执行流程。



### 绘制步骤

1. **创建SVG画布**

   ```svg

   <svg width="1000" height="400" xmlns="http://www.w3.org/2000/svg">

   ```



2. **绘制标题**

   ```svg

   <text x="500" y="30" text-anchor="middle" font-family="Arial" font-size="18" font-weight="bold">动作类型执行流程</text>

   ```



3. **绘制设备控制流程**

   ```svg

   <!-- 设备控制 -->

   <rect x="50" y="60" width="120" height="40" rx="5" fill="#f0f5ff" stroke="#2f54eb" stroke-width="1"/>

   <text x="110" y="85" text-anchor="middle" font-family="Arial" font-size="14">目标解析</text>



   <rect x="200" y="60" width="120" height="40" rx="5" fill="#f0f5ff" stroke="#2f54eb" stroke-width="1"/>

   <text x="260" y="85" text-anchor="middle" font-family="Arial" font-size="14">表达式计算</text>



   <rect x="350" y="60" width="120" height="40" rx="5" fill="#f0f5ff" stroke="#2f54eb" stroke-width="1"/>

   <text x="410" y="85" text-anchor="middle" font-family="Arial" font-size="14">位操作处理</text>



   <rect x="200" y="140" width="120" height="40" rx="5" fill="#f0f5ff" stroke="#2f54eb" stroke-width="1"/>

   <text x="260" y="165" text-anchor="middle" font-family="Arial" font-size="14">设备写入</text>



   <path d="M170 80 L200 80" stroke="#2f54eb" stroke-width="2" marker-end="url(#arrowhead)"/>

   <path d="M320 80 L350 80" stroke="#2f54eb" stroke-width="2" marker-end="url(#arrowhead)"/>

   <path d="M410 100 L410 140" stroke="#2f54eb" stroke-width="2" marker-end="url(#arrowhead)"/>

   <path d="M350 160 L200 160" stroke="#2f54eb" stroke-width="2" marker-end="url(#arrowhead)"/>

   ```



4. **绘制MQTT发布流程**

   ```svg

   <!-- MQTT发布 -->

   <rect x="550" y="60" width="120" height="40" rx="5" fill="#f6ffed" stroke="#52c41a" stroke-width="1"/>

   <text x="610" y="85" text-anchor="middle" font-family="Arial" font-size="14">配置解析</text>



   <rect x="700" y="60" width="120" height="40" rx="5" fill="#f6ffed" stroke="#52c41a" stroke-width="1"/>

   <text x="760" y="85" text-anchor="middle" font-family="Arial" font-size="14">消息生成</text>



   <rect x="850" y="60" width="120" height="40" rx="5" fill="#f6ffed" stroke="#52c41a" stroke-width="1"/>

   <text x="910" y="85" text-anchor="middle" font-family="Arial" font-size="14">MQTT发布</text>



   <path d="M670 80 L700 80" stroke="#52c41a" stroke-width="2" marker-end="url(#arrowhead)"/>

   <path d="M820 80 L850 80" stroke="#52c41a" stroke-width="2" marker-end="url(#arrowhead)"/>

   ```



5. **绘制HTTP请求流程**

   ```svg

   <!-- HTTP请求 -->

   <rect x="550" y="140" width="120" height="40" rx="5" fill="#fff7e6" stroke="#fa8c16" stroke-width="1"/>

   <text x="610" y="165" text-anchor="middle" font-family="Arial" font-size="14">配置解析</text>



   <rect x="700" y="140" width="120" height="40" rx="5" fill="#fff7e6" stroke="#fa8c16" stroke-width="1"/>

   <text x="760" y="165" text-anchor="middle" font-family="Arial" font-size="14">请求生成</text>



   <rect x="850" y="140" width="120" height="40" rx="5" fill="#fff7e6" stroke="#fa8c16" stroke-width="1"/>

   <text x="910" y="165" text-anchor="middle" font-family="Arial" font-size="14">HTTP请求</text>



   <path d="M670 160 L700 160" stroke="#fa8c16" stroke-width="2" marker-end="url(#arrowhead)"/>

   <path d="M820 160 L850 160" stroke="#fa8c16" stroke-width="2" marker-end="url(#arrowhead)"/>

   ```



6. **绘制数据库存储流程**

   ```svg

   <!-- 数据库存储 -->

   <rect x="550" y="220" width="120" height="40" rx="5" fill="#f0f0f0" stroke="#333" stroke-width="1"/>

   <text x="610" y="245" text-anchor="middle" font-family="Arial" font-size="14">配置解析</text>



   <rect x="700" y="220" width="120" height="40" rx="5" fill="#f0f0f0" stroke="#333" stroke-width="1"/>

   <text x="760" y="245" text-anchor="middle" font-family="Arial" font-size="14">数据准备</text>



   <rect x="850" y="220" width="120" height="40" rx="5" fill="#f0f0f0" stroke="#333" stroke-width="1"/>

   <text x="910" y="245" text-anchor="middle" font-family="Arial" font-size="14">存储执行</text>



   <path d="M670 240 L700 240" stroke="#333" stroke-width="2" marker-end="url(#arrowhead)"/>

   <path d="M820 240 L850 240" stroke="#333" stroke-width="2" marker-end="url(#arrowhead)"/>

   ```



7. **添加箭头标记**

   ```svg

   <defs>

     <marker id="arrowhead" markerWidth="10" markerHeight="7" refX="9" refY="3.5" orient="auto">

       <polygon points="0 0, 10 3.5, 0 7" fill="#333"/>

     </marker>

   </defs>

   ```



8. **关闭SVG标签**

   ```svg

   </svg>

   ```



## 4. 系统拓扑图SVG绘制步骤



### 流程概述

展示边缘计算系统的整体架构，包括边缘设备层、数据采集层、边缘计算核心层和外部系统层。



### 绘制步骤

1. **创建SVG画布**

   ```svg

   <svg width="1000" height="500" xmlns="http://www.w3.org/2000/svg">

   ```



2. **绘制标题**

   ```svg

   <text x="500" y="30" text-anchor="middle" font-family="Arial" font-size="18" font-weight="bold">边缘计算系统拓扑</text>

   ```



3. **绘制系统边界**

   ```svg

   <rect x="50" y="50" width="900" height="400" rx="10" fill="#f9f9f9" stroke="#333" stroke-width="2"/>

   <text x="500" y="80" text-anchor="middle" font-family="Arial" font-size="16" font-weight="bold">边缘计算系统</text>

   ```



4. **绘制边缘设备层**

   ```svg

   <!-- 边缘设备层 -->

   <rect x="70" y="120" width="200" height="120" rx="5" fill="#e6f7ff" stroke="#1890ff" stroke-width="1"/>

   <text x="170" y="140" text-anchor="middle" font-family="Arial" font-size="14" font-weight="bold">边缘设备层</text>



   <rect x="90" y="170" width="80" height="30" rx="3" fill="#ffffff" stroke="#1890ff" stroke-width="1"/>

   <text x="130" y="190" text-anchor="middle" font-family="Arial" font-size="12">传感器设备</text>



   <rect x="190" y="170" width="80" height="30" rx="3" fill="#ffffff" stroke="#1890ff" stroke-width="1"/>

   <text x="230" y="190" text-anchor="middle" font-family="Arial" font-size="12">执行器设备</text>

   ```



5. **绘制数据采集层**

   ```svg

   <!-- 数据采集层 -->

   <rect x="300" y="120" width="200" height="120" rx="5" fill="#f6ffed" stroke="#52c41a" stroke-width="1"/>

   <text x="400" y="140" text-anchor="middle" font-family="Arial" font-size="14" font-weight="bold">数据采集层</text>



   <rect x="320" y="170" width="80" height="30" rx="3" fill="#ffffff" stroke="#52c41a" stroke-width="1"/>

   <text x="360" y="190" text-anchor="middle" font-family="Arial" font-size="12">通道管理</text>



   <rect x="420" y="170" width="80" height="30" rx="3" fill="#ffffff" stroke="#52c41a" stroke-width="1"/>

   <text x="460" y="190" text-anchor="middle" font-family="Arial" font-size="12">协议适配</text>

   ```



6. **绘制边缘计算核心层**

   ```svg

   <!-- 边缘计算核心层 -->

   <rect x="530" y="120" width="200" height="120" rx="5" fill="#fff7e6" stroke="#fa8c16" stroke-width="1"/>

   <text x="630" y="140" text-anchor="middle" font-family="Arial" font-size="14" font-weight="bold">边缘计算核心层</text>



   <rect x="550" y="170" width="80" height="30" rx="3" fill="#ffffff" stroke="#fa8c16" stroke-width="1"/>

   <text x="590" y="190" text-anchor="middle" font-family="Arial" font-size="12">数据管道</text>



   <rect x="650" y="170" width="80" height="30" rx="3" fill="#ffffff" stroke="#fa8c16" stroke-width="1"/>

   <text x="690" y="190" text-anchor="middle" font-family="Arial" font-size="12">规则引擎</text>

   ```



7. **绘制外部系统层**

   ```svg

   <!-- 外部系统层 -->

   <rect x="760" y="120" width="190" height="120" rx="5" fill="#fff1f0" stroke="#f5222d" stroke-width="1"/>

   <text x="855" y="140" text-anchor="middle" font-family="Arial" font-size="14" font-weight="bold">外部系统层</text>



   <rect x="780" y="170" width="80" height="30" rx="3" fill="#ffffff" stroke="#f5222d" stroke-width="1"/>

   <text x="820" y="190" text-anchor="middle" font-family="Arial" font-size="12">MQTT</text>



   <rect x="880" y="170" width="80" height="30" rx="3" fill="#ffffff" stroke="#f5222d" stroke-width="1"/>

   <text x="920" y="190" text-anchor="middle" font-family="Arial" font-size="12">HTTP</text>

   ```



8. **绘制连接线**

   ```svg

   <!-- 边缘设备到数据采集 -->

   <path d="M270 180 L300 180" stroke="#333" stroke-width="2" marker-end="url(#arrowhead)"/>



   <!-- 数据采集到边缘计算 -->

   <path d="M500 180 L530 180" stroke="#333" stroke-width="2" marker-end="url(#arrowhead)"/>



   <!-- 边缘计算到外部系统 -->

   <path d="M730 180 L760 180" stroke="#333" stroke-width="2" marker-end="url(#arrowhead)"/>

   ```



9. **添加箭头标记**

   ```svg

   <defs>

     <marker id="arrowhead" markerWidth="10" markerHeight="7" refX="9" refY="3.5" orient="auto">

       <polygon points="0 0, 10 3.5, 0 7" fill="#333"/>

     </marker>

   </defs>

   ```



10. **关闭SVG标签**

    ```svg

    </svg>

    ```



## 5. 数据流向拓扑SVG绘制步骤



### 流程概述

展示边缘计算系统中数据的流动方向，包括设备数据采集、边缘计算处理、动作执行和外部系统交互。



### 绘制步骤

1. **创建SVG画布**

   ```svg

   <svg width="1000" height="300" xmlns="http://www.w3.org/2000/svg">

   ```



2. **绘制标题**

   ```svg

   <text x="500" y="30" text-anchor="middle" font-family="Arial" font-size="18" font-weight="bold">数据流向拓扑</text>

   ```



3. **绘制四个主要节点**

   ```svg

   <!-- 设备数据 -->

   <rect x="50" y="80" width="120" height="40" rx="5" fill="#e6f7ff" stroke="#1890ff" stroke-width="1"/>

   <text x="110" y="105" text-anchor="middle" font-family="Arial" font-size="14">设备数据</text>



   <!-- 数据采集 -->

   <rect x="220" y="80" width="120" height="40" rx="5" fill="#f6ffed" stroke="#52c41a" stroke-width="1"/>

   <text x="280" y="105" text-anchor="middle" font-family="Arial" font-size="14">数据采集</text>



   <!-- 边缘计算 -->

   <rect x="390" y="80" width="120" height="40" rx="5" fill="#fff7e6" stroke="#fa8c16" stroke-width="1"/>

   <text x="450" y="105" text-anchor="middle" font-family="Arial" font-size="14">边缘计算</text>



   <!-- 动作执行 -->

   <rect x="560" y="80" width="120" height="40" rx="5" fill="#fff1f0" stroke="#f5222d" stroke-width="1"/>

   <text x="620" y="105" text-anchor="middle" font-family="Arial" font-size="14">动作执行</text>



   <!-- 外部系统 -->

   <rect x="730" y="80" width="120" height="40" rx="5" fill="#f0f5ff" stroke="#2f54eb" stroke-width="1"/>

   <text x="790" y="105" text-anchor="middle" font-family="Arial" font-size="14">外部系统</text>

   ```



4. **绘制数据流向**

   ```svg

   <!-- 主数据流 -->

   <path d="M170 100 L220 100" stroke="#333" stroke-width="2" marker-end="url(#arrowhead)"/>

   <path d="M340 100 L390 100" stroke="#333" stroke-width="2" marker-end="url(#arrowhead)"/>

   <path d="M510 100 L560 100" stroke="#333" stroke-width="2" marker-end="url(#arrowhead)"/>

   <path d="M680 100 L730 100" stroke="#333" stroke-width="2" marker-end="url(#arrowhead)"/>



   <!-- 反馈流 -->

   <path d="M790 120 L790 180 L110 180 L110 120" stroke="#999" stroke-width="1" stroke-dasharray="5,5" marker-end="url(#arrowhead)"/>

   ```



5. **添加箭头标记**

   ```svg

   <defs>

     <marker id="arrowhead" markerWidth="10" markerHeight="7" refX="9" refY="3.5" orient="auto">

       <polygon points="0 0, 10 3.5, 0 7" fill="#333"/>

     </marker>

   </defs>

   ```



6. **关闭SVG标签**

   ```svg

   </svg>

   ```



## 6. 规则配置拓扑SVG绘制步骤



### 流程概述

展示边缘计算规则的配置、加载、执行和存储流程。



### 绘制步骤

1. **创建SVG画布**

   ```svg

   <svg width="800" height="300" xmlns="http://www.w3.org/2000/svg">

   ```



2. **绘制标题**

   ```svg

   <text x="400" y="30" text-anchor="middle" font-family="Arial" font-size="18" font-weight="bold">规则配置拓扑</text>

   ```



3. **绘制规则配置流程**

   ```svg

   <!-- 规则文件 -->

   <rect x="50" y="80" width="120" height="40" rx="5" fill="#f0f0f0" stroke="#333" stroke-width="1"/>

   <text x="110" y="105" text-anchor="middle" font-family="Arial" font-size="14">规则文件</text>



   <!-- 规则加载 -->

   <rect x="220" y="80" width="120" height="40" rx="5" fill="#f0f0f0" stroke="#333" stroke-width="1"/>

   <text x="280" y="105" text-anchor="middle" font-family="Arial" font-size="14">规则加载</text>



   <!-- 规则解析 -->

   <rect x="390" y="80" width="120" height="40" rx="5" fill="#f0f0f0" stroke="#333" stroke-width="1"/>

   <text x="450" y="105" text-anchor="middle" font-family="Arial" font-size="14">规则解析</text>



   <!-- 规则索引 -->

   <rect x="560" y="80" width="120" height="40" rx="5" fill="#f0f0f0" stroke="#333" stroke-width="1"/>

   <text x="620" y="105" text-anchor="middle" font-family="Arial" font-size="14">规则索引</text>



   <!-- 规则执行 -->

   <rect x="390" y="160" width="120" height="40" rx="5" fill="#f0f0f0" stroke="#333" stroke-width="1"/>

   <text x="450" y="185" text-anchor="middle" font-family="Arial" font-size="14">规则执行</text>

   ```



4. **绘制连接线**

   ```svg

   <!-- 规则配置流程 -->

   <path d="M170 100 L220 100" stroke="#333" stroke-width="2" marker-end="url(#arrowhead)"/>

   <path d="M340 100 L390 100" stroke="#333" stroke-width="2" marker-end="url(#arrowhead)"/>

   <path d="M510 100 L560 100" stroke="#333" stroke-width="2" marker-end="url(#arrowhead)"/>



   <!-- 规则执行反馈 -->

   <path d="M620 120 L620 180 L450 180" stroke="#999" stroke-width="1" stroke-dasharray="5,5" marker-end="url(#arrowhead)"/>

   <path d="M450 200 L450 240 L110 240 L110 120" stroke="#999" stroke-width="1" stroke-dasharray="5,5" marker-end="url(#arrowhead)"/>

   ```



5. **添加箭头标记**

   ```svg

   <defs>

     <marker id="arrowhead" markerWidth="10" markerHeight="7" refX="9" refY="3.5" orient="auto">

       <polygon points="0 0, 10 3.5, 0 7" fill="#333"/>

     </marker>

   </defs>

   ```



6. **关闭SVG标签**

   ```svg

   </svg>

   ```



## 7. 工作流执行流程SVG绘制步骤



### 流程概述

展示边缘计算工作流的执行流程，包括序列执行、延迟执行、条件检查和回退处理。



### 绘制步骤

1. **创建SVG画布**

   ```svg

   <svg width="800" height="300" xmlns="http://www.w3.org/2000/svg">

   ```



2. **绘制标题**

   ```svg

   <text x="400" y="30" text-anchor="middle" font-family="Arial" font-size="18" font-weight="bold">工作流执行流程</text>

   ```



3. **绘制工作流步骤**

   ```svg

   <!-- 序列执行 -->

   <rect x="50" y="80" width="120" height="40" rx="5" fill="#e6f7ff" stroke="#1890ff" stroke-width="1"/>

   <text x="110" y="105" text-anchor="middle" font-family="Arial" font-size="14">序列执行</text>



   <!-- 延迟执行 -->

   <rect x="220" y="80" width="120" height="40" rx="5" fill="#f6ffed" stroke="#52c41a" stroke-width="1"/>

   <text x="280" y="105" text-anchor="middle" font-family="Arial" font-size="14">延迟执行</text>



   <!-- 条件检查 -->

   <rect x="390" y="80" width="120" height="40" rx="5" fill="#fff7e6" stroke="#fa8c16" stroke-width="1"/>

   <text x="450" y="105" text-anchor="middle" font-family="Arial" font-size="14">条件检查</text>



   <!-- 结果验证 -->

   <rect x="390" y="160" width="120" height="40" rx="5" fill="#fff1f0" stroke="#f5222d" stroke-width="1"/>

   <text x="450" y="185" text-anchor="middle" font-family="Arial" font-size="14">结果验证</text>



   <!-- 回退处理 -->

   <rect x="220" y="160" width="120" height="40" rx="5" fill="#f0f5ff" stroke="#2f54eb" stroke-width="1"/>

   <text x="280" y="185" text-anchor="middle" font-family="Arial" font-size="14">回退处理</text>

   ```



4. **绘制连接线**

   ```svg

   <!-- 工作流正向流程 -->

   <path d="M170 100 L220 100" stroke="#333" stroke-width="2" marker-end="url(#arrowhead)"/>

   <path d="M340 100 L390 100" stroke="#333" stroke-width="2" marker-end="url(#arrowhead)"/>

   <path d="M450 120 L450 160" stroke="#333" stroke-width="2" marker-end="url(#arrowhead)"/>



   <!-- 回退流程 -->

   <path d="M450 180 L280 180" stroke="#999" stroke-width="1" stroke-dasharray="5,5" marker-end="url(#arrowhead)"/>

   ```



5. **添加箭头标记**

   ```svg

   <defs>

     <marker id="arrowhead" markerWidth="10" markerHeight="7" refX="9" refY="3.5" orient="auto">

       <polygon points="0 0, 10 3.5, 0 7" fill="#333"/>

     </marker>

   </defs>

   ```

