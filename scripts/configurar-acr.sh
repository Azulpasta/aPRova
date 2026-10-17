#!/usr/bin/env bash
set -Eeuo pipefail

GRUPO="rg-aprova"
JOB="aprova-sandbox-teste"
APLICACAO="aprova-github"
CREDENCIAL_FEDERADA="github-main"
REPOSITORIO_GITHUB="Azulpasta/aPRova"
EMISSOR_OIDC="https://token.actions.githubusercontent.com"
AUDIENCIA_OIDC="api://AzureADTokenExchange"
TENTATIVAS=10
ESPERA_SEGUNDOS=30

export PYTHONWARNINGS=ignore
export AZURE_CORE_ONLY_SHOW_ERRORS=true

PASSO="validação dos argumentos"
trap 'echo "falhou no passo: ${PASSO}" >&2' ERR

uso() {
	echo "uso: $0 <nome_do_registro> [--plano]" >&2
	echo "  nome_do_registro: 5 a 50 caracteres, só letras minúsculas e números" >&2
	echo "  --plano: só consulta e mostra o que seria criado, sem criar nada" >&2
	exit 2
}

[ "$#" -ge 1 ] && [ "$#" -le 2 ] || uso
REGISTRO="$1"
PLANO=false
if [ "$#" -eq 2 ]; then
	[ "$2" = "--plano" ] || uso
	PLANO=true
fi
[[ "$REGISTRO" =~ ^[a-z0-9]{5,50}$ ]] || uso

CRIADOS=0

relatar() {
	echo "  ${1}: ${2}"
}

criar_ou_planejar() {
	local descricao="$1"
	shift
	if [ "$PLANO" = true ]; then
		relatar "$descricao" "seria criado"
		return 0
	fi
	"$@" >/dev/null
	CRIADOS=$((CRIADOS + 1))
	relatar "$descricao" "criado"
}

aguardar_propagacao() {
	local descricao="$1"
	shift
	local tentativa saida
	for tentativa in $(seq 1 "$TENTATIVAS"); do
		if saida=$("$@" 2>&1); then
			return 0
		fi
		if ! grep -qiE 'AuthorizationFailed|Forbidden|does not have authorization|PrincipalNotFound|does not exist in the directory|UNAUTHORIZED|denied' <<<"$saida"; then
			echo "$saida" >&2
			return 1
		fi
		echo "  ${descricao}: permissão ainda propagando, tentativa ${tentativa} de ${TENTATIVAS}, aguardando ${ESPERA_SEGUNDOS}s" >&2
		sleep "$ESPERA_SEGUNDOS"
	done
	echo "$saida" >&2
	return 1
}

atribuir_papel() {
	local descricao="$1" objeto="$2" papel="$3" escopo="$4"
	local existentes
	existentes=$(az role assignment list --assignee "$objeto" --scope "$escopo" --role "$papel" --query 'length(@)' -o tsv)
	if [ "$existentes" -gt 0 ]; then
		relatar "$descricao" "já existia"
		return 0
	fi
	if [ "$PLANO" = true ]; then
		relatar "$descricao" "seria criado"
		return 0
	fi
	aguardar_propagacao "$descricao" az role assignment create \
		--assignee-object-id "$objeto" --assignee-principal-type ServicePrincipal \
		--role "$papel" --scope "$escopo"
	CRIADOS=$((CRIADOS + 1))
	relatar "$descricao" "criado"
}

PASSO="0/5 pré-condições"
echo "[$PASSO]"
az account show >/dev/null || { echo "  faça az login antes" >&2; exit 1; }
gh auth status >/dev/null 2>&1 || { echo "  faça gh auth login antes" >&2; exit 1; }
az group show -n "$GRUPO" >/dev/null
az containerapp job show -n "$JOB" -g "$GRUPO" >/dev/null
ASSINATURA=$(az account show --query id -o tsv)
TENANT=$(az account show --query tenantId -o tsv)
relatar "az, gh, grupo ${GRUPO} e job ${JOB}" "ok"

PASSO="1/5 registro ${REGISTRO}"
echo "[$PASSO]"
if [ "$(az provider show -n Microsoft.ContainerRegistry --query registrationState -o tsv)" = "Registered" ]; then
	relatar "provedor Microsoft.ContainerRegistry na assinatura" "já existia"
else
	criar_ou_planejar "provedor Microsoft.ContainerRegistry na assinatura" \
		az provider register -n Microsoft.ContainerRegistry --wait
fi
if az acr show -n "$REGISTRO" -g "$GRUPO" >/dev/null 2>&1; then
	relatar "registro ${REGISTRO} (Basic) em ${GRUPO}" "já existia"
else
	if [ "$(az acr check-name -n "$REGISTRO" --query nameAvailable -o tsv)" != "true" ]; then
		echo "  o nome ${REGISTRO} já está em uso na Azure por outro registro; escolha outro" >&2
		exit 1
	fi
	criar_ou_planejar "registro ${REGISTRO} (Basic) em ${GRUPO}" \
		az acr create -n "$REGISTRO" -g "$GRUPO" --sku Basic --admin-enabled false
fi
ID_REGISTRO=$(az acr show -n "$REGISTRO" -g "$GRUPO" --query id -o tsv 2>/dev/null || echo "")

PASSO="2/5 aplicação ${APLICACAO}, service principal e AcrPush"
echo "[$PASSO]"
APP_ID=$(az ad app list --display-name "$APLICACAO" --query '[0].appId' -o tsv)
if [ -n "$APP_ID" ]; then
	relatar "aplicação ${APLICACAO}" "já existia"
else
	criar_ou_planejar "aplicação ${APLICACAO}, sem segredo de cliente" \
		az ad app create --display-name "$APLICACAO" --sign-in-audience AzureADMyOrg
	APP_ID=$(az ad app list --display-name "$APLICACAO" --query '[0].appId' -o tsv)
fi

SP_OBJETO=""
if [ -n "$APP_ID" ]; then
	SP_OBJETO=$(az ad sp list --filter "appId eq '${APP_ID}'" --query '[0].id' -o tsv)
	if [ -n "$SP_OBJETO" ]; then
		relatar "service principal de ${APLICACAO}" "já existia"
	else
		criar_ou_planejar "service principal de ${APLICACAO}" az ad sp create --id "$APP_ID"
		SP_OBJETO=$(az ad sp list --filter "appId eq '${APP_ID}'" --query '[0].id' -o tsv)
	fi
else
	relatar "service principal de ${APLICACAO}" "seria criado"
fi

if [ -n "$SP_OBJETO" ] && [ -n "$ID_REGISTRO" ]; then
	atribuir_papel "papel AcrPush de ${APLICACAO} no registro" "$SP_OBJETO" AcrPush "$ID_REGISTRO"
	OUTROS_PAPEIS=$(az role assignment list --assignee "$SP_OBJETO" --all --query "[?roleDefinitionName!='AcrPush'].roleDefinitionName" -o tsv)
	if [ -n "$OUTROS_PAPEIS" ]; then
		echo "  ${APLICACAO} tem papéis além de AcrPush (${OUTROS_PAPEIS//$'\n'/, }); remova-os antes de seguir" >&2
		exit 1
	fi
	relatar "${APLICACAO} sem nenhum papel além de AcrPush" "ok"
else
	relatar "papel AcrPush de ${APLICACAO} no registro" "seria criado"
fi

PASSO="3/5 credencial federada OIDC"
echo "[$PASSO]"
PREFIXO_OIDC=$(gh api "repos/${REPOSITORIO_GITHUB}/actions/oidc/customization/sub" \
	--jq 'if .use_default then (.sub_claim_prefix // "") else "personalizado" end')
if [ "$PREFIXO_OIDC" = "personalizado" ]; then
	echo "  o repositório usa um template de subject OIDC personalizado; revise a credencial manualmente" >&2
	exit 1
fi
SUJEITO_OIDC="${PREFIXO_OIDC:-repo:${REPOSITORIO_GITHUB}}:ref:refs/heads/main"
PARAMETROS_OIDC="{\"name\":\"${CREDENCIAL_FEDERADA}\",\"issuer\":\"${EMISSOR_OIDC}\",\"subject\":\"${SUJEITO_OIDC}\",\"audiences\":[\"${AUDIENCIA_OIDC}\"]}"

if [ -n "$APP_ID" ]; then
	SUJEITO_EXISTENTE=$(az ad app federated-credential list --id "$APP_ID" --query "[?name=='${CREDENCIAL_FEDERADA}'].subject | [0]" -o tsv)
else
	SUJEITO_EXISTENTE=""
fi
if [ "$SUJEITO_EXISTENTE" = "$SUJEITO_OIDC" ]; then
	relatar "credencial ${CREDENCIAL_FEDERADA} para ${SUJEITO_OIDC}" "já existia"
elif [ -n "$SUJEITO_EXISTENTE" ]; then
	criar_ou_planejar "credencial ${CREDENCIAL_FEDERADA} atualizada de ${SUJEITO_EXISTENTE} para ${SUJEITO_OIDC}" \
		az ad app federated-credential update --id "$APP_ID" --federated-credential-id "$CREDENCIAL_FEDERADA" --parameters "$PARAMETROS_OIDC"
else
	criar_ou_planejar "credencial ${CREDENCIAL_FEDERADA} para ${SUJEITO_OIDC}" \
		az ad app federated-credential create --id "$APP_ID" --parameters "$PARAMETROS_OIDC"
fi

PASSO="4/5 identidade do job ${JOB} e AcrPull"
echo "[$PASSO]"
TIPO_IDENTIDADE=$(az containerapp job identity show -n "$JOB" -g "$GRUPO" --query type -o tsv 2>/dev/null || echo "None")
if [[ "$TIPO_IDENTIDADE" == *SystemAssigned* ]]; then
	relatar "identidade gerenciada do job" "já existia"
else
	criar_ou_planejar "identidade gerenciada do job (system-assigned)" \
		az containerapp job identity assign -n "$JOB" -g "$GRUPO" --system-assigned
fi
IDENTIDADE_DO_JOB=$(az containerapp job identity show -n "$JOB" -g "$GRUPO" --query principalId -o tsv 2>/dev/null || echo "")

if [ -n "$IDENTIDADE_DO_JOB" ] && [ -n "$ID_REGISTRO" ]; then
	atribuir_papel "papel AcrPull da identidade do job no registro" "$IDENTIDADE_DO_JOB" AcrPull "$ID_REGISTRO"
else
	relatar "papel AcrPull da identidade do job no registro" "seria criado"
fi

SERVIDOR="${REGISTRO}.azurecr.io"
CONFIGURADO=$(az containerapp job registry list -n "$JOB" -g "$GRUPO" --query "[?server=='${SERVIDOR}'].identity | [0]" -o tsv 2>/dev/null || echo "")
if [ "$CONFIGURADO" = "system" ]; then
	relatar "job puxando de ${SERVIDOR} com a identidade" "já existia"
elif [ "$PLANO" = true ]; then
	relatar "job puxando de ${SERVIDOR} com a identidade" "seria criado"
else
	aguardar_propagacao "registro no job" az containerapp job registry set -n "$JOB" -g "$GRUPO" \
		--server "$SERVIDOR" --identity system
	CRIADOS=$((CRIADOS + 1))
	relatar "job puxando de ${SERVIDOR} com a identidade" "criado"
fi

PASSO="5/5 variável e segredos no GitHub"
echo "[$PASSO]"
VARIAVEL_ATUAL=$(gh variable get ACR_NOME -R "$REPOSITORIO_GITHUB" 2>/dev/null || echo "")
if [ "$VARIAVEL_ATUAL" = "$REGISTRO" ]; then
	relatar "variável ACR_NOME" "já existia"
else
	criar_ou_planejar "variável ACR_NOME" gh variable set ACR_NOME -R "$REPOSITORIO_GITHUB" --body "$REGISTRO"
fi

SEGREDOS_EXISTENTES=$(gh secret list -R "$REPOSITORIO_GITHUB" --json name -q '.[].name')
gravar_segredo() {
	local nome="$1" valor="$2"
	if [ "$PLANO" != true ] && [ -z "$valor" ]; then
		echo "  ${nome} ficaria vazio; nada foi gravado" >&2
		exit 1
	fi
	if [ "$PLANO" = true ]; then
		if grep -qx "$nome" <<<"$SEGREDOS_EXISTENTES"; then
			relatar "segredo ${nome}" "já existia, seria regravado com o valor atual"
		else
			relatar "segredo ${nome}" "seria criado"
		fi
		return 0
	fi
	if grep -qx "$nome" <<<"$SEGREDOS_EXISTENTES"; then
		printf '%s' "$valor" | gh secret set "$nome" -R "$REPOSITORIO_GITHUB" >/dev/null
		relatar "segredo ${nome}" "já existia, regravado com o valor atual"
	else
		printf '%s' "$valor" | gh secret set "$nome" -R "$REPOSITORIO_GITHUB" >/dev/null
		CRIADOS=$((CRIADOS + 1))
		relatar "segredo ${nome}" "criado"
	fi
}
gravar_segredo AZURE_CLIENT_ID "${APP_ID:-}"
gravar_segredo AZURE_TENANT_ID "$TENANT"
gravar_segredo AZURE_SUBSCRIPTION_ID "$ASSINATURA"

PASSO="concluído"
if [ "$PLANO" = true ]; then
	echo "plano concluído, nada foi criado"
else
	echo "concluído: ${CRIADOS} recurso(s) criado(s)"
fi
