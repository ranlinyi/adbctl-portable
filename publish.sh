#!/usr/bin/env bash
# publish.sh — 一条命令把源码推到 GitHub，并把便携版二进制发到 Release
#
#   GITHUB_TOKEN=ghp_xxx ./publish.sh
#
# 可选环境变量：
#   GITHUB_OWNER   默认 ranlinyi
#   REPO_NAME      默认 adbctl-portable
#   TAG            默认 v1.0.0
#   RELEASE_TITLE  默认 adbctl 1.0.0（自包含单文件便携版）
#
# 需要：git、curl、一个对仓库有 repo/contents 写权限的 PAT。
set -euo pipefail

OWNER="${GITHUB_OWNER:-ranlinyi}"
REPO="${REPO_NAME:-adbctl-portable}"
TAG="${TAG:-v1.0.0}"
TITLE="${RELEASE_TITLE:-adbctl 1.0.0（自包含单文件便携版）}"
API="https://api.github.com"
UPLOADS="https://uploads.github.com"

if [ -z "${GITHUB_TOKEN:-}" ]; then
  echo "错误：请设置 GITHUB_TOKEN，例如：" >&2
  echo "  GITHUB_TOKEN=ghp_xxx ./publish.sh" >&2
  exit 1
fi

ROOT="$(cd "$(dirname "$0")" && pwd)"
cd "$ROOT"
mkdir -p .build
JSON="$ROOT/.build/publish.json"

get_id()    { grep -o '"id"[[:space:]]*:[[:space:]]*[0-9][0-9]*' | head -1 | grep -o '[0-9][0-9]*$' || true; }
get_login() { grep -o '"login"[[:space:]]*:[[:space:]]*"[^"]*"' | head -1 | sed 's/.*"\([^"]*\)"$/\1/' || true; }

echo "== 0/5 校验 token =="
ME="$(curl -fsS -H "Authorization: Bearer $GITHUB_TOKEN" -H "Accept: application/vnd.github+json" "$API/user")" \
  || { echo "token 无效或网络不通" >&2; exit 1; }
echo "已认证：$(printf '%s' "$ME" | get_login)"

echo "== 1/5 创建或复用仓库 $OWNER/$REPO =="
code="$(curl -sS -o "$JSON" -w '%{http_code}' \
  -H "Authorization: Bearer $GITHUB_TOKEN" -H "Accept: application/vnd.github+json" \
  "$API/repos/$OWNER/$REPO")"
if [ "$code" = "200" ]; then
  echo "仓库已存在"
else
  curl -fsS -o "$JSON" -X POST \
    -H "Authorization: Bearer $GITHUB_TOKEN" -H "Accept: application/vnd.github+json" \
    -H "Content-Type: application/json" \
    "$API/user/repos" \
    -d "{\"name\":\"$REPO\",\"private\":false,\"description\":\"adbctl 自包含单文件版：内嵌 adb + scrcpy，支持 Windows/Linux\"}"
  echo "仓库已创建"
fi

echo "== 2/5 推送 main =="
git remote remove origin 2>/dev/null || true
git remote add origin "https://github.com/$OWNER/$REPO.git"
git -c credential.helper= \
  -c credential.helper='!f() { echo username=x-access-token; echo "password=$GITHUB_TOKEN"; }; f' \
  push -u origin main

echo "== 3/5 创建或复用 Release $TAG =="
REL="$(curl -sS -H "Authorization: Bearer $GITHUB_TOKEN" -H "Accept: application/vnd.github+json" \
  "$API/repos/$OWNER/$REPO/releases/tags/$TAG")"
REL_ID="$(printf '%s' "$REL" | get_id)"
if [ -z "$REL_ID" ]; then
  REL="$(curl -fsS -X POST \
    -H "Authorization: Bearer $GITHUB_TOKEN" -H "Accept: application/vnd.github+json" \
    -H "Content-Type: application/json" \
    "$API/repos/$OWNER/$REPO/releases" \
    -d "{\"tag_name\":\"$TAG\",\"name\":\"$TITLE\",\"draft\":false}")"
  REL_ID="$(printf '%s' "$REL" | get_id)"
  echo "Release 已创建 id=$REL_ID"
else
  echo "Release 已存在 id=$REL_ID"
fi

echo "== 4/5 上传便携版二进制 =="
for f in dist/adbctl-linux-x86_64 dist/adbctl-windows-x86_64.exe dist/SHA256SUMS; do
  if [ ! -f "$f" ]; then
    echo "缺少 $f，请先运行 ./build.sh" >&2
    exit 1
  fi
  name="$(basename "$f")"
  echo "  上传 $name ..."
  curl -fsS -X POST \
    -H "Authorization: Bearer $GITHUB_TOKEN" \
    -H "Accept: application/vnd.github+json" \
    -H "Content-Type: application/octet-stream" \
    --data-binary @"$f" \
    "$UPLOADS/repos/$OWNER/$REPO/releases/$REL_ID/assets?name=$name" >/dev/null
done

echo "== 5/5 完成 =="
echo "仓库：  https://github.com/$OWNER/$REPO"
echo "发行版：https://github.com/$OWNER/$REPO/releases/tag/$TAG"
