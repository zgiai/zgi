# TODO

1. 实现文件读取接口：
   a. 在 app/common/utils/file_operations.py 中实现文件操作工具函数
   b. 在 app/services/file_service.py 中创建文件服务逻辑
   c. 在 app/api/v1/endpoints/file_operations.py 中开发文件操作 API 端点

2. 更新目录结构：
   a. 在 app/Exceptions/ 目录中创建自定义异常处理器
   b. 将配置从 app/core/config.py 移动到 config/ 目录并更新相关导入
   c. 在 database/ 目录中创建必要的迁移和种子文件
   d. 将路由定义从 app/api/v1/router.py 移动到 routes/api.py

3. 更新 app/main.py 以反映新的目录结构

4. 根据需要，更新其他文件中的导入语句

列出所有团队
获取团队详情
更新团队信息
删除团队
成员管理
邀请管理

2.2 应用管理
2.2.1 应用创建
- 支持创建新应用
- 设置应用名称和描述
- 选择应用类型（对话式、生成式等）
- 配置应用访问权限