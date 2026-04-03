#!/bin/bash
# Print KubeOpenCode pixel logos in terminal
# KUBE = Kubernetes blue (#326CE5), OPENCODE = dark gray
# Left padding (P) for screenshot margin

# ANSI color codes
K8S_BLUE='\033[38;2;50;108;229m'  # #326CE5
OC_GRAY='\033[38;2;101;99;99m'    # #656363
RESET='\033[0m'
P="          "  # 10-char left margin for screenshots

echo ""
echo ""
echo "${P}1. KubeOpenCode Logo (Stacked)"
echo "${P}─────────────────────────────"
echo ""
echo ""

echo -e "${P}${K8S_BLUE}                  ██╗  ██╗██╗   ██╗██████╗ ███████╗${RESET}"
echo -e "${P}${K8S_BLUE}                  ██║ ██╔╝██║   ██║██╔══██╗██╔════╝${RESET}"
echo -e "${P}${K8S_BLUE}                  █████╔╝ ██║   ██║██████╔╝█████╗  ${RESET}"
echo -e "${P}${K8S_BLUE}                  ██╔═██╗ ██║   ██║██╔══██╗██╔══╝  ${RESET}"
echo -e "${P}${K8S_BLUE}                  ██║  ██╗╚██████╔╝██████╔╝███████╗${RESET}"
echo -e "${P}${K8S_BLUE}                  ╚═╝  ╚═╝ ╚═════╝ ╚═════╝ ╚══════╝${RESET}"
echo -e "${P}${OC_GRAY} ██████╗ ██████╗ ███████╗███╗   ██╗ ██████╗ ██████╗ ██████╗ ███████╗${RESET}"
echo -e "${P}${OC_GRAY}██╔═══██╗██╔══██╗██╔════╝████╗  ██║██╔════╝██╔═══██╗██╔══██╗██╔════╝${RESET}"
echo -e "${P}${OC_GRAY}██║   ██║██████╔╝█████╗  ██╔██╗ ██║██║     ██║   ██║██║  ██║█████╗  ${RESET}"
echo -e "${P}${OC_GRAY}██║   ██║██╔═══╝ ██╔══╝  ██║╚██╗██║██║     ██║   ██║██║  ██║██╔══╝  ${RESET}"
echo -e "${P}${OC_GRAY}╚██████╔╝██║     ███████╗██║ ╚████║╚██████╗╚██████╔╝██████╔╝███████╗${RESET}"
echo -e "${P}${OC_GRAY} ╚═════╝ ╚═╝     ╚══════╝╚═╝  ╚═══╝ ╚═════╝ ╚═════╝ ╚═════╝ ╚══════╝${RESET}"

echo ""
echo ""
echo ""
echo ""
echo "${P}2. KubeOpenCode Logo (Inline)"
echo "${P}─────────────────────────────"
echo ""
echo ""

echo -e "${P}${K8S_BLUE}██╗  ██╗██╗   ██╗██████╗ ███████╗${RESET}    ${OC_GRAY} ██████╗ ██████╗ ███████╗███╗   ██╗ ██████╗ ██████╗ ██████╗ ███████╗${RESET}"
echo -e "${P}${K8S_BLUE}██║ ██╔╝██║   ██║██╔══██╗██╔════╝${RESET}    ${OC_GRAY}██╔═══██╗██╔══██╗██╔════╝████╗  ██║██╔════╝██╔═══██╗██╔══██╗██╔════╝${RESET}"
echo -e "${P}${K8S_BLUE}█████╔╝ ██║   ██║██████╔╝█████╗  ${RESET}    ${OC_GRAY}██║   ██║██████╔╝█████╗  ██╔██╗ ██║██║     ██║   ██║██║  ██║█████╗  ${RESET}"
echo -e "${P}${K8S_BLUE}██╔═██╗ ██║   ██║██╔══██╗██╔══╝  ${RESET}    ${OC_GRAY}██║   ██║██╔═══╝ ██╔══╝  ██║╚██╗██║██║     ██║   ██║██║  ██║██╔══╝  ${RESET}"
echo -e "${P}${K8S_BLUE}██║  ██╗╚██████╔╝██████╔╝███████╗${RESET}    ${OC_GRAY}╚██████╔╝██║     ███████╗██║ ╚████║╚██████╗╚██████╔╝██████╔╝███████╗${RESET}"
echo -e "${P}${K8S_BLUE}╚═╝  ╚═╝ ╚═════╝ ╚═════╝ ╚══════╝${RESET}    ${OC_GRAY} ╚═════╝ ╚═╝     ╚══════╝╚═╝  ╚═══╝ ╚═════╝ ╚═════╝ ╚═════╝ ╚══════╝${RESET}"

echo ""
echo ""
echo ""
echo ""
echo "${P}3. K Icon"
echo "${P}─────────"
echo ""
echo ""

echo -e "${P}${K8S_BLUE}██╗  ██╗${RESET}"
echo -e "${P}${K8S_BLUE}██║ ██╔╝${RESET}"
echo -e "${P}${K8S_BLUE}█████╔╝ ${RESET}"
echo -e "${P}${K8S_BLUE}██╔═██╗ ${RESET}"
echo -e "${P}${K8S_BLUE}██║  ██╗${RESET}"
echo -e "${P}${K8S_BLUE}╚═╝  ╚═╝${RESET}"

echo ""
echo ""
