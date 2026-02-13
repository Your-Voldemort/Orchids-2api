# 需求文档

## 简介

对 CodeFreeMax Go 管理面板进行全面视觉重设计。保持所有现有功能不变（4个页面：账号管理、配置管理、模型管理、使用教程），重新设计整体视觉风格、配色方案和布局。技术约束：继续使用 Go `embed.FS` 嵌入式前端架构，纯 HTML/CSS/JS，无构建步骤。

## 术语表

- **Design_System**: CSS 变量定义的设计令牌系统，包含颜色、间距、圆角、阴影等
- **Sidebar**: 左侧固定导航栏组件，包含菜单项和统计信息
- **Content_Card**: 内容区域的卡片容器组件
- **Modal**: 模态弹窗组件，用于表单输入和确认操作
- **Toast**: 底部右侧的临时通知提示组件
- **Glass_Morphism**: 当前使用的毛玻璃视觉效果（backdrop-filter blur）
- **Page_Template**: Go 模板引擎渲染的页面 HTML 文件
- **Stat_Card**: 统计数据展示卡片组件
- **Toolbar**: 表格上方的操作栏，包含筛选、按钮等
- **Toggle_Switch**: 开关切换组件

## 需求

### 需求 1：设计令牌系统重构

**用户故事：** 作为开发者，我希望有一套全新的设计令牌系统，以便统一控制整个面板的视觉风格。

#### 验收标准

1. THE Design_System SHALL 定义全新的配色方案，包含主色、强调色、背景色、文字色和边框色的完整 CSS 变量集
2. THE Design_System SHALL 定义间距令牌（spacing tokens），包含至少 4 个层级的间距值
3. THE Design_System SHALL 定义圆角令牌（border-radius tokens），包含 small、medium、large 三个层级
4. THE Design_System SHALL 定义阴影令牌（shadow tokens），包含至少 2 个层级的阴影效果
5. THE Design_System SHALL 定义过渡动画令牌（transition tokens），确保交互动画一致
6. THE Design_System SHALL 保留 Outfit 和 JetBrains Mono 字体配置，或替换为同等质量的字体组合

### 需求 2：整体布局重设计

**用户故事：** 作为管理员，我希望面板有全新的布局风格，以获得更现代、更清晰的使用体验。

#### 验收标准

1. THE Sidebar SHALL 采用全新的视觉样式，包括新的背景色、边框处理和菜单项样式
2. WHEN 菜单项被选中时，THE Sidebar SHALL 以新的视觉方式高亮当前活动页面
3. THE Sidebar SHALL 保持固定定位和统计信息底栏功能不变
4. THE Content_Card SHALL 采用新的背景色、边框和阴影样式
5. WHEN 页面加载时，THE Page_Template SHALL 展示新的页面标题区域样式
6. THE Page_Template SHALL 在移动端（≤900px）保持响应式布局能力

### 需求 3：组件视觉重设计

**用户故事：** 作为管理员，我希望所有 UI 组件（按钮、输入框、表格、标签等）有统一的新视觉风格。

#### 验收标准

1. THE Design_System SHALL 为按钮组件（primary、outline、danger、icon）定义新的样式，包括颜色、圆角和悬停效果
2. THE Design_System SHALL 为表单输入组件（input、select、textarea）定义新的样式，包括背景色、边框和聚焦状态
3. THE Design_System SHALL 为表格组件定义新的样式，包括表头、行、悬停效果和边框
4. THE Design_System SHALL 为标签/徽章组件（tag、platform-badge）定义新的配色方案
5. THE Design_System SHALL 为 Toggle_Switch 组件定义新的视觉样式
6. THE Design_System SHALL 为进度条组件定义新的颜色和样式
7. THE Design_System SHALL 为 Stat_Card 组件定义新的视觉样式，包括图标区域和数值展示

### 需求 4：弹窗与通知重设计

**用户故事：** 作为管理员，我希望弹窗和通知有更精致的视觉效果。

#### 验收标准

1. THE Modal SHALL 采用新的背景色、边框、圆角和阴影样式
2. THE Modal SHALL 保持现有的打开/关闭动画或采用新的过渡动画
3. THE Toast SHALL 采用新的视觉样式，与整体设计风格一致
4. WHEN Modal 打开时，THE Modal SHALL 显示新样式的遮罩层背景

### 需求 5：登录页面重设计

**用户故事：** 作为管理员，我希望登录页面有全新的视觉风格，与管理面板整体设计一致。

#### 验收标准

1. THE Page_Template SHALL 为登录页面定义新的背景效果（替换当前的 blob 动画或重新设计）
2. THE Page_Template SHALL 为登录卡片定义新的视觉样式
3. THE Page_Template SHALL 保持登录表单的所有功能（用户名、密码、密码显示切换、提交）不变

### 需求 6：技术约束保持

**用户故事：** 作为开发者，我希望重设计不破坏现有的技术架构和功能。

#### 验收标准

1. THE Design_System SHALL 仅修改 CSS 样式文件和 HTML 模板文件，不修改任何 JavaScript 逻辑
2. THE Page_Template SHALL 保持所有 Go 模板语法（{{define}}、{{template}}、{{.Field}}）不变
3. THE Page_Template SHALL 保持所有 HTML 元素的 id 属性不变，确保 JavaScript 绑定正常工作
4. THE Page_Template SHALL 保持所有 onclick 事件处理器不变
5. THE Design_System SHALL 保持所有 CSS 类名不变，或在修改类名时同步更新所有引用
6. IF CSS 类名被修改，THEN THE Design_System SHALL 确保所有模板文件和 JavaScript 文件中的引用同步更新
7. THE Page_Template SHALL 保持 `embed.FS` 文件结构不变（css/main.css、js/*.js、templates/**）
