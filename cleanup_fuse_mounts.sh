#!/bin/bash

# 清理所有僵尸 FUSE 挂载点的脚本
# 使用方法: ./cleanup_fuse_mounts.sh

echo "正在检查 FUSE 挂载点..."

# 查找所有包含 "Transport endpoint is not connected" 错误的挂载点
ZOMBIE_MOUNTS=$(mount | grep fuse | grep -E "offsetfs|syncmate|cgofs_test" | awk '{print $3}')

if [ -z "$ZOMBIE_MOUNTS" ]; then
    echo "没有发现可疑的 FUSE 挂载点。"
    exit 0
fi

echo "发现以下 FUSE 挂载点："
echo "$ZOMBIE_MOUNTS"

echo "正在尝试卸载..."
for mount_point in $ZOMBIE_MOUNTS; do
    echo "卸载: $mount_point"
    fusermount -u "$mount_point" 2>/dev/null || fusermount3 -u "$mount_point" 2>/dev/null
    if [ $? -eq 0 ]; then
        echo "  ✓ 成功卸载: $mount_point"
    else
        echo "  ✗ 卸载失败: $mount_point"
    fi
done

# 清理测试目录
echo "清理临时测试目录..."
find /tmp -name "cgofs_test_*" -type d -user $(whoami) -exec rm -rf {} + 2>/dev/null || true

echo "清理完成！"
echo "当前 FUSE 挂载状态:"
mount | grep fuse || echo "没有活跃的 FUSE 挂载点。"
