# 实施计划：前端视觉重设计

## 概述

将 CodeFreeMax 管理面板从当前的深色毛玻璃风格重设计为现代简洁的深色主题。所有修改仅涉及 CSS 和 HTML 模板文件，JavaScript 逻辑和 Go 模板语法保持不变。

## 任务

- [x] 1. 重写设计令牌系统和基础样式
  - [x] 1.1 重写 `web/static/css/main.css` 中的 `:root` CSS 变量定义，替换为新的配色方案、间距、圆角、阴影和过渡令牌
    - 替换所有颜色变量（primary、accent、bg、text、border）
    - 添加间距令牌（space-xs/sm/md/lg/xl/2xl）
    - 更新圆角令牌和阴影令牌
    - 添加过渡动画令牌（transition-fast/transition/transition-slow）
    - _Requirements: 1.1, 1.2, 1.3, 1.4, 1.5, 1.6_
  - [x] 1.2 更新 `main.css` 中的 Reset & Base Styles 部分，包括 body 背景、滚动条和基础排版
    - 更新 body 背景色和背景渐变效果
    - 更新滚动条样式
    - _Requirements: 1.1_

- [x] 2. 重设计侧边栏和布局
  - [x] 2.1 更新 `main.css` 中的 Sidebar 样式部分
    - 更新侧边栏背景色、边框
    - 更新 Logo 区域样式
    - 更新菜单项样式和活动状态高亮
    - 更新统计底栏样式
    - _Requirements: 2.1, 2.2, 2.3_
  - [x] 2.2 更新 `web/templates/partials/sidebar.html` 的视觉元素（如 emoji 图标、内联样式），保持所有 Go 模板变量和 onclick 事件不变
    - _Requirements: 2.1, 2.3, 6.2, 6.4_
  - [x] 2.3 更新 `main.css` 中的 Main Content Area 样式
    - 更新内容区域内边距和动画
    - 更新页面标题区域样式
    - _Requirements: 2.4, 2.5_

- [x] 3. 重设计核心 UI 组件
  - [x] 3.1 更新 `main.css` 中的 Stats Card 样式
    - 更新卡片背景、边框、阴影和悬停效果
    - 更新图标区域样式
    - _Requirements: 3.7_
  - [x] 3.2 更新 `main.css` 中的 Table 和 Toolbar 样式
    - 更新表头样式（去掉 text-transform: uppercase）
    - 更新行样式和悬停效果
    - 更新 toolbar 背景和 filter-tabs 样式
    - 更新 token-text 和 tag/badge 样式
    - _Requirements: 3.3, 3.4_
  - [x] 3.3 更新 `main.css` 中的 Button 样式
    - 更新 btn-primary、btn-outline、btn-danger-outline、btn-icon 样式
    - 使用新的主色渐变和悬停效果
    - _Requirements: 3.1_
  - [x] 3.4 更新 `main.css` 中的 Form Input 和 Toggle Switch 样式
    - 更新输入框背景色、边框和聚焦状态
    - 更新 Toggle 开关颜色
    - _Requirements: 3.2, 3.5_
  - [x] 3.5 更新 `main.css` 中的 Progress Bar 样式
    - 更新进度条颜色和发光效果
    - _Requirements: 3.6_

- [x] 4. 重设计弹窗和通知
  - [x] 4.1 更新 `main.css` 中的 Modal 样式
    - 更新遮罩层背景
    - 更新弹窗卡片背景、边框、圆角
    - 更新弹窗头部和关闭按钮样式
    - _Requirements: 4.1, 4.2, 4.4_
  - [x] 4.2 更新 `main.css` 中的 Toast 样式
    - 更新通知背景、边框和阴影
    - _Requirements: 4.3_

- [x] 5. 重设计登录页面
  - [x] 5.1 更新 `main.css` 中的 Login Page 样式
    - 更新登录卡片背景和阴影
    - 更新 blob 动画效果或替换为新的背景效果
    - _Requirements: 5.1, 5.2_
  - [x] 5.2 更新 `web/static/login.html` 中的内联样式，使其与新设计令牌一致，保持所有表单功能和 JavaScript 不变
    - _Requirements: 5.3, 6.1_

- [x] 6. 更新页面模板内联样式
  - [x] 6.1 更新 `web/templates/pages/accounts.html` 中的内联样式，使其与新设计令牌一致，保持所有 id 属性和 onclick 事件不变
    - _Requirements: 6.3, 6.4_
  - [x] 6.2 更新 `web/templates/pages/config.html` 中的内联样式，使其与新设计令牌一致
    - _Requirements: 6.3, 6.4_
  - [x] 6.3 更新 `web/templates/pages/models.html` 中的内联样式
    - _Requirements: 6.3, 6.4_
  - [x] 6.4 更新 `web/templates/pages/tutorial.html` 中的内联样式
    - _Requirements: 6.3, 6.4_
  - [x] 6.5 更新 `web/templates/components/modals/` 下的 3 个弹窗模板的内联样式
    - _Requirements: 6.3, 6.4_

- [x] 7. 更新响应式样式
  - [x] 7.1 更新 `main.css` 中的 media query（≤900px）部分，确保新样式在移动端正常显示
    - _Requirements: 2.6_

- [x] 8. 检查点 - 验证功能完整性
  - 确保所有测试通过，如有问题请询问用户。
  - 在浏览器中逐页检查 4 个管理页面和登录页面
  - 验证所有按钮点击、表单提交、弹窗、Toast 通知正常工作
  - 验证响应式布局在 900px 以下正常显示

- [ ]* 9. 编写属性测试验证技术约束
  - [ ]* 9.1 编写属性测试：JavaScript 文件完整性
    - **Property 1: JavaScript 文件完整性**
    - 验证 common.js、accounts.js、config.js、models.js 未被修改
    - **Validates: Requirements 6.1**
  - [ ]* 9.2 编写属性测试：Go 模板语法保留
    - **Property 2: Go 模板语法保留**
    - 验证所有 HTML 模板中的 Go 模板指令集合未变
    - **Validates: Requirements 6.2**
  - [ ]* 9.3 编写属性测试：HTML id 属性和事件处理器保留
    - **Property 3: HTML id 属性保留**
    - **Property 4: 事件处理器保留**
    - 验证所有模板中的 id 属性和 onclick 事件处理器集合未变
    - **Validates: Requirements 6.3, 6.4**
  - [ ]* 9.4 编写属性测试：CSS 类名引用一致性
    - **Property 5: CSS 类名引用一致性**
    - 验证 JS 文件中引用的 CSS 类名在 main.css 中有定义
    - **Validates: Requirements 6.5, 6.6**

- [x] 10. 最终检查点
  - 确保所有测试通过，如有问题请询问用户。

## 备注

- 标记 `*` 的任务为可选任务，可跳过以加快 MVP 进度
- 每个任务引用了具体的需求编号以确保可追溯性
- 检查点确保增量验证
- 属性测试验证技术约束的正确性
- 所有修改严格限制在 CSS 和 HTML 模板文件，不触碰 JavaScript 和 Go 代码
