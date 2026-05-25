#!/usr/bin/env bash
set -euo pipefail

# Kubernetes 清单一致性检查。
# 目标：离线渲染 all-in-one.yaml 与拆分清单，确认两种部署入口包含同一组资源。

usage() {
  cat <<'EOF'
用法:
  scripts/ops/k8s-manifest-check.sh [--k8s-dir ../deploy/k8s]

参数:
  --k8s-dir DIR   K8s 清单目录，默认 ../deploy/k8s
  -h, --help      显示帮助
EOF
}

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "缺少依赖命令: $1" >&2
    exit 2
  fi
}

render_all_in_one() {
  local k8s_dir="$1"
  local tmp_dir="$2"
  mkdir -p "$tmp_dir"
  cp "${k8s_dir}/all-in-one.yaml" "${tmp_dir}/all-in-one.yaml"
  cat >"${tmp_dir}/kustomization.yaml" <<'EOF'
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
resources:
  - all-in-one.yaml
EOF
  kubectl kustomize "$tmp_dir"
}

resource_keys() {
  awk '
    function emit() {
      if (kind != "" && name != "") {
        if (namespace == "") {
          namespace = "_cluster"
        }
        print apiVersion "/" kind "/" namespace "/" name
      }
    }
    /^---$/ {
      emit()
      apiVersion = ""
      kind = ""
      name = ""
      namespace = ""
      inMetadata = 0
      next
    }
    /^apiVersion:/ {
      apiVersion = $2
      next
    }
    /^kind:/ {
      kind = $2
      next
    }
    /^metadata:/ {
      inMetadata = 1
      next
    }
    /^[^ ]/ && $0 !~ /^metadata:/ {
      inMetadata = 0
    }
    inMetadata && /^  name:/ {
      name = $2
      next
    }
    inMetadata && /^  namespace:/ {
      namespace = $2
      next
    }
    END {
      emit()
    }
  ' | sort
}

K8S_DIR="../deploy/k8s"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --k8s-dir)
      K8S_DIR="${2:-}"; shift 2 ;;
    -h|--help)
      usage; exit 0 ;;
    *)
      echo "未知参数: $1" >&2
      usage
      exit 2 ;;
  esac
done

require_cmd kubectl

if [[ ! -d "$K8S_DIR" ]]; then
  echo "K8s 清单目录不存在: $K8S_DIR" >&2
  exit 2
fi
if [[ ! -f "${K8S_DIR}/all-in-one.yaml" || ! -f "${K8S_DIR}/kustomization.yaml" ]]; then
  echo "K8s 清单目录必须包含 all-in-one.yaml 和 kustomization.yaml" >&2
  exit 2
fi

tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"' EXIT

render_all_in_one "$K8S_DIR" "$tmp_dir/all" >"$tmp_dir/all-rendered.yaml"
kubectl kustomize "$K8S_DIR" >"$tmp_dir/split-rendered.yaml"

resource_keys <"$tmp_dir/all-rendered.yaml" >"$tmp_dir/all.keys"
resource_keys <"$tmp_dir/split-rendered.yaml" >"$tmp_dir/split.keys"

if ! diff -u "$tmp_dir/all.keys" "$tmp_dir/split.keys"; then
  echo "K8s 单文件入口与拆分清单资源集合不一致" >&2
  exit 1
fi

if ! diff -u "$tmp_dir/all-rendered.yaml" "$tmp_dir/split-rendered.yaml"; then
  echo "K8s 单文件入口与拆分清单渲染结果不一致，请同步 all-in-one.yaml 和拆分清单" >&2
  exit 1
fi

echo "K8s 清单资源集合一致，共 $(wc -l <"$tmp_dir/all.keys" | tr -d ' ') 个资源"
