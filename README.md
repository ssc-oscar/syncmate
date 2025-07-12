# OffsetFS - FUSE 偏移文件系统

OffsetFS 是一个基于 FUSE 的文件系统，允许您创建虚拟文件，这些文件映射到现有文件的特定偏移量和大小。

## 功能特性

- **偏移访问**: 从源文件的指定偏移量开始读写
- **大小限制**: 可以限制虚拟文件的大小
- **只读模式**: 支持只读访问模式
- **直接访问**: 当 offset=0 且 size=0 时，直接访问整个文件
- **文件创建**: 支持创建不存在的源文件
- **追加写入**: 支持在文件末尾追加写入，自动扩展文件大小
- **灵活的大小控制**: 
  - size=0 时允许文件动态增长
  - size>0 时限制虚拟文件的最大大小
- **JSONL 配置**: 通过 JSONL 格式的配置文件定义文件映射
- **错误处理**: 完善的错误检查和处理机制

## 编译

```bash
go mod tidy
go build -o offsetfs fs.go
```

## 使用方法

### 1. 创建配置文件

创建一个 JSONL 格式的配置文件，每行定义一个虚拟文件映射：

```jsonl
{"virtual_path": "example.txt", "source_path": "/path/to/source.txt", "offset": 0, "size": 0, "read_only": false}
{"virtual_path": "part1.txt", "source_path": "/path/to/source.txt", "offset": 100, "size": 200, "read_only": true}
{"virtual_path": "tail.txt", "source_path": "/path/to/source.txt", "offset": 1000, "size": 0, "read_only": true}
```

### 2. 挂载文件系统

```bash
# 创建挂载点
mkdir /tmp/offsetfs_mount

# 挂载文件系统
./offsetfs -config config.jsonl /tmp/offsetfs_mount
```

### 3. 使用文件系统

```bash
# 列出虚拟文件
ls -la /tmp/offsetfs_mount/

# 读取虚拟文件
cat /tmp/offsetfs_mount/example.txt

# 写入虚拟文件（如果不是只读模式）
echo "new content" > /tmp/offsetfs_mount/example.txt
```

### 4. 卸载文件系统

```bash
# 使用 Ctrl-C 停止程序，或在另一个终端中：
umount /tmp/offsetfs_mount
```

## 配置参数

每个 JSONL 配置行包含以下字段：

- `virtual_path`: 虚拟文件系统中的文件名（不支持路径分隔符，仅支持根目录）
- `source_path`: 源文件的完整路径（如果不存在会自动创建）
- `offset`: 从源文件开始读取的偏移量（字节数）
- `size`: 虚拟文件的大小
  - `0`: 从 offset 到文件末尾，允许动态增长
  - `>0`: 限制虚拟文件的最大大小
- `read_only`: 是否为只读模式

## 写入行为说明

### 文件创建
- 如果源文件不存在，系统会自动创建文件（除非是只读模式）
- 会自动创建必要的父目录

### 追加写入
- **直接访问模式** (offset=0, size=0): 支持在文件任意位置写入，包括超出当前文件大小的位置
- **偏移访问模式**: 
  - 当 `size=0` 时，允许从偏移量开始无限制写入
  - 当 `size>0` 时，写入不能超过配置的大小限制
- 写入超出当前文件大小时，文件会自动扩展

### 示例写入场景

```jsonl
# 完全自由的文件访问，支持任意位置读写和追加
{"virtual_path": "free_access.txt", "source_path": "/path/to/file.txt", "offset": 0, "size": 0, "read_only": false}

# 从偏移量100开始，无大小限制，可以追加
{"virtual_path": "offset_append.txt", "source_path": "/path/to/file.txt", "offset": 100, "size": 0, "read_only": false}

# 从偏移量100开始，最大200字节，受限制的写入
{"virtual_path": "limited_write.txt", "source_path": "/path/to/file.txt", "offset": 100, "size": 200, "read_only": false}

# 只读访问，不允许任何写入
{"virtual_path": "readonly.txt", "source_path": "/path/to/file.txt", "offset": 0, "size": 0, "read_only": true}
```

## 命令行选项

- `-config`: JSONL 配置文件路径（必需）
- `-debug`: 启用调试输出
- `-allow-other`: 允许其他用户访问文件系统

## 使用场景

1. **日志文件分片**: 将大型日志文件分解为多个小文件进行分析
2. **数据提取**: 从二进制文件中提取特定部分
3. **文件合并视图**: 将文件的不同部分组合成新的虚拟文件
4. **只读访问控制**: 为文件的某些部分提供只读访问
5. **测试和开发**: 模拟不同大小的文件进行测试
6. **增量文件写入**: 在现有文件的特定位置进行增量更新
7. **临时文件创建**: 创建临时的虚拟文件而不影响原始文件结构
8. **文件追加**: 在文件末尾安全地追加内容

## 注意事项

- 当前版本仅支持根目录下的文件（不支持子目录）
- 写入操作会直接影响源文件
- 在只读模式下，任何写入操作都会被拒绝
- 源文件如果不存在会自动创建（非只读模式）
- 写入超出文件当前大小时，文件会自动扩展
- 配置中的 `size` 参数仅在大于0时起限制作用
- 在 macOS 上可能需要安装 macFUSE

## 示例

使用提供的示例配置：

```bash
# 编译程序
go build -o offsetfs fs.go

# 创建挂载点
mkdir /tmp/test_mount

# 挂载
./offsetfs -config config.jsonl /tmp/test_mount

# 在另一个终端中测试
ls -la /tmp/test_mount/
cat /tmp/test_mount/example.txt
cat /tmp/test_mount/example_offset.txt

# 测试写入（example.txt 允许写入，其他是只读）
echo "Hello World" > /tmp/test_mount/example.txt

# 测试追加写入
echo "Appended content" >> /tmp/test_mount/example.txt

# 测试文件创建（使用 test_config.jsonl）
./offsetfs -config test_config.jsonl /tmp/test_mount

# 创建新文件
echo "This is a new file" > /tmp/test_mount/new_file.txt
cat /tmp/test_mount/new_file.txt

# 测试偏移写入
echo "Content at offset 50" > /tmp/test_mount/offset_write.txt
```

### 高级使用示例

```bash
# 1. 创建一个大文件的分片视图
echo '{"virtual_path": "log_part1.txt", "source_path": "/var/log/large.log", "offset": 0, "size": 1048576, "read_only": true}' > split_config.jsonl
echo '{"virtual_path": "log_part2.txt", "source_path": "/var/log/large.log", "offset": 1048576, "size": 1048576, "read_only": true}' >> split_config.jsonl

# 2. 创建一个可追加的日志文件
echo '{"virtual_path": "append_log.txt", "source_path": "/tmp/app.log", "offset": 0, "size": 0, "read_only": false}' > append_config.jsonl

# 3. 创建受限制的编辑视图
echo '{"virtual_path": "edit_window.txt", "source_path": "/path/to/document.txt", "offset": 1000, "size": 500, "read_only": false}' > edit_config.jsonl
```

## 错误处理

程序包含完善的错误处理机制：

- 配置文件语法错误检查
- 源文件存在性验证
- 偏移量和大小合理性检查
- 虚拟路径重复检查
- 文件访问权限检查
- I/O 操作错误处理
