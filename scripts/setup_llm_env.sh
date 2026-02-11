#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CORE_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
ENV_FILE="${CORE_DIR}/.env"
ENV_EXAMPLE_FILE="${CORE_DIR}/.env.example"

prompt_yes_no() {
  local prompt="${1}"
  local default="${2:-y}"
  local answer
  while true; do
    if [[ "${default}" == "y" ]]; then
      read -r -p "${prompt} [Y/n]: " answer || true
      answer="${answer:-y}"
    else
      read -r -p "${prompt} [y/N]: " answer || true
      answer="${answer:-n}"
    fi
    case "${answer}" in
      y|Y|yes|YES) return 0 ;;
      n|N|no|NO) return 1 ;;
      *) echo "Please answer y or n." ;;
    esac
  done
}

prompt_line() {
  local prompt="${1}"
  local default="${2:-}"
  local answer
  if [[ -n "${default}" ]]; then
    read -r -p "${prompt} [${default}]: " answer || true
    echo "${answer:-${default}}"
  else
    read -r -p "${prompt}: " answer || true
    echo "${answer}"
  fi
}

prompt_secret() {
  local prompt="${1}"
  local answer
  read -r -s -p "${prompt}: " answer || true
  echo
  echo "${answer}"
}

ensure_env_file() {
  if [[ -f "${ENV_FILE}" ]]; then
    return
  fi
  if [[ -f "${ENV_EXAMPLE_FILE}" ]]; then
    cp "${ENV_EXAMPLE_FILE}" "${ENV_FILE}"
  else
    touch "${ENV_FILE}"
  fi
}

set_env_value() {
  local key="${1}"
  local value="${2}"
  local escaped_value
  escaped_value="$(printf '%s' "${value}" | sed -e 's/[\/&]/\\&/g')"
  if grep -qE "^${key}=" "${ENV_FILE}"; then
    sed -i "s/^${key}=.*/${key}=${escaped_value}/" "${ENV_FILE}"
  else
    printf '\n%s=%s\n' "${key}" "${value}" >> "${ENV_FILE}"
  fi
}

echo "Insightify LLM API setup"
echo "Target env file: ${ENV_FILE}"
echo

ensure_env_file

if prompt_yes_no "Configure Gemini key?" "y"; then
  gemini_key="$(prompt_secret "Enter GEMINI_API_KEY (leave empty to skip update)")"
  if [[ -n "${gemini_key}" ]]; then
    set_env_value "GEMINI_API_KEY" "${gemini_key}"
  fi
fi

if prompt_yes_no "Configure Groq key?" "y"; then
  groq_key="$(prompt_secret "Enter GROQ_API_KEY (leave empty to skip update)")"
  if [[ -n "${groq_key}" ]]; then
    set_env_value "GROQ_API_KEY" "${groq_key}"
  fi
fi

if prompt_yes_no "Configure global rate limit (LLM_RPS / LLM_BURST)?" "n"; then
  llm_rps="$(prompt_line "LLM_RPS (example: 2, empty to disable)" "")"
  llm_burst="$(prompt_line "LLM_BURST (example: 2)" "")"
  set_env_value "LLM_RPS" "${llm_rps}"
  set_env_value "LLM_BURST" "${llm_burst}"
fi

if prompt_yes_no "Configure Gemini-specific rate limit (GEMINI_RPS / GEMINI_BURST)?" "n"; then
  gemini_rps="$(prompt_line "GEMINI_RPS" "")"
  gemini_burst="$(prompt_line "GEMINI_BURST" "")"
  set_env_value "GEMINI_RPS" "${gemini_rps}"
  set_env_value "GEMINI_BURST" "${gemini_burst}"
fi

echo
echo "Setup completed."
echo "Updated: ${ENV_FILE}"
