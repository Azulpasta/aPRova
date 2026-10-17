#!/usr/bin/env bash
set -Eeuo pipefail

GRUPO="rg-aprova"
JOB="aprova-sandbox-teste"
APLICACAO="aprova-github"
CREDENCIAL_FEDERADA="github-main"
REPOSITORIO_GITHUB="Azulpasta/aPRova"
EMISSOR_OIDC="https://token.actions.githubusercontent.com"
AUDIENCIA_OIDC="api://AzureADTokenExchange"
PLANO_DO_REGISTRO="Basic"
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

abortar() {
	echo "  ${1}" >&2
	exit 1
}

executar_ou_planejar() {
	local descricao="$1" efeito="$2"
	shift 2
	if [ "$PLANO" = true ]; then
		relatar "$descricao" "seria ${efeito}"
		return 0
	fi
	"$@" >/dev/null
	CRIADOS=$((CRIADOS + 1))
	relatar "$descricao" "$efeito"
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

ler_identificador() {
	local descricao="$1"
	shift
	local tentativa valor
	for tentativa in $(seq 1 "$TENTATIVAS"); do
		valor=$("$@" 2>/dev/null || true)
		if [ -n "$valor" ] && [ "$valor" != "None" ]; then
			printf '%s' "$valor"
			return 0
		fi
		[ "$PLANO" = true ] && return 0
		echo "  ${descricao}: ainda não disponível, tentativa ${tentativa} de ${TENTATIVAS}, aguardando ${ESPERA_SEGUNDOS}s" >&2
		sleep "$ESPERA_SEGUNDOS"
	done
	abortar "${descricao} não ficou disponível; nada que dependa dele foi criado"
}

atribuir_papel() {
	local descricao="$1" objeto="$2" papel="$3" escopo="$4"
	if [ -z "$objeto" ] || [ -z "$escopo" ]; then
		relatar "$descricao" "seria criado"
		return 0
	fi
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
az account show >/dev/null || abortar "faça az login antes"
gh auth status >/dev/null 2>&1 || abortar "faça gh auth login antes"
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
	executar_ou_planejar "provedor Microsoft.ContainerRegistry na assinatura" "registrado" \
		az provider register -n Microsoft.ContainerRegistry --wait
fi

if az acr show -n "$REGISTRO" -g "$GRUPO" >/dev/null 2>&1; then
	relatar "registro ${REGISTRO} em ${GRUPO}" "já existia"
	PLANO_ATUAL=$(az acr show -n "$REGISTRO" -g "$GRUPO" --query sku.name -o tsv)
	[ "$PLANO_ATUAL" = "$PLANO_DO_REGISTRO" ] || abortar "o registro ${REGISTRO} está no plano ${PLANO_ATUAL}, não ${PLANO_DO_REGISTRO}; revise antes de seguir"
	if [ "$(az acr show -n "$REGISTRO" -g "$GRUPO" --query adminUserEnabled -o tsv)" = "true" ]; then
		executar_ou_planejar "usuário administrador do registro" "desligado" \
			az acr update -n "$REGISTRO" -g "$GRUPO" --admin-enabled false
	else
		relatar "usuário administrador do registro desligado" "ok"
	fi
else
	if [ "$(az acr check-name -n "$REGISTRO" --query nameAvailable -o tsv)" != "true" ]; then
		abortar "o nome ${REGISTRO} já está em uso na Azure por outro registro; escolha outro"
	fi
	executar_ou_planejar "registro ${REGISTRO} (${PLANO_DO_REGISTRO}, sem administrador) em ${GRUPO}" "criado" \
		az acr create -n "$REGISTRO" -g "$GRUPO" --sku "$PLANO_DO_REGISTRO" --admin-enabled false
fi
ID_REGISTRO=$(ler_identificador "id do registro ${REGISTRO}" az acr show -n "$REGISTRO" -g "$GRUPO" --query id -o tsv)

PASSO="2/5 aplicação ${APLICACAO}, service principal e AcrPush"
echo "[$PASSO]"
QUANTAS_APLICACOES=$(az ad app list --display-name "$APLICACAO" --query 'length(@)' -o tsv)
[ "$QUANTAS_APLICACOES" -le 1 ] || abortar "há ${QUANTAS_APLICACOES} aplicações chamadas ${APLICACAO}; deixe só uma antes de seguir"
if [ "$QUANTAS_APLICACOES" -eq 1 ]; then
	relatar "aplicação ${APLICACAO}" "já existia"
else
	executar_ou_planejar "aplicação ${APLICACAO}, sem segredo de cliente" "criado" \
		az ad app create --display-name "$APLICACAO" --sign-in-audience AzureADMyOrg
fi
APP_ID=$(ler_identificador "appId de ${APLICACAO}" az ad app list --display-name "$APLICACAO" --query '[0].appId' -o tsv)

SP_OBJETO=""
if [ -n "$APP_ID" ]; then
	if [ -n "$(az ad sp list --filter "appId eq '${APP_ID}'" --query '[0].id' -o tsv)" ]; then
		relatar "service principal de ${APLICACAO}" "já existia"
	else
		executar_ou_planejar "service principal de ${APLICACAO}" "criado" az ad sp create --id "$APP_ID"
	fi
	SP_OBJETO=$(ler_identificador "service principal de ${APLICACAO}" az ad sp list --filter "appId eq '${APP_ID}'" --query '[0].id' -o tsv)
else
	relatar "service principal de ${APLICACAO}" "seria criado"
fi

if [ -n "$SP_OBJETO" ]; then
	OUTROS_PAPEIS=$(az role assignment list --assignee "$SP_OBJETO" --all --query "[?roleDefinitionName!='AcrPush'].roleDefinitionName" -o tsv)
	[ -z "$OUTROS_PAPEIS" ] || abortar "${APLICACAO} tem papéis além de AcrPush (${OUTROS_PAPEIS//$'\n'/, }); remova-os antes de seguir"
	relatar "${APLICACAO} sem nenhum papel além de AcrPush" "ok"
fi
atribuir_papel "papel AcrPush de ${APLICACAO} no registro" "$SP_OBJETO" AcrPush "$ID_REGISTRO"

PASSO="3/5 credencial federada OIDC"
echo "[$PASSO]"
PREFIXO_OIDC=$(gh api "repos/${REPOSITORIO_GITHUB}/actions/oidc/customization/sub" \
	--jq 'if .use_default then (.sub_claim_prefix // "") else "personalizado" end')
[ "$PREFIXO_OIDC" != "personalizado" ] || abortar "o repositório usa um template de subject OIDC personalizado; revise a credencial manualmente"
SUJEITO_OIDC="${PREFIXO_OIDC:-repo:${REPOSITORIO_GITHUB}}:ref:refs/heads/main"
PARAMETROS_OIDC="{\"name\":\"${CREDENCIAL_FEDERADA}\",\"issuer\":\"${EMISSOR_OIDC}\",\"subject\":\"${SUJEITO_OIDC}\",\"audiences\":[\"${AUDIENCIA_OIDC}\"]}"

CREDENCIAL_ATUAL=""
if [ -n "$APP_ID" ]; then
	CREDENCIAL_ATUAL=$(az ad app federated-credential list --id "$APP_ID" \
		--query "[?name=='${CREDENCIAL_FEDERADA}'].join('|', [issuer, subject, join(',', audiences)]) | [0]" -o tsv)
fi
CREDENCIAL_ESPERADA="${EMISSOR_OIDC}|${SUJEITO_OIDC}|${AUDIENCIA_OIDC}"
if [ "$CREDENCIAL_ATUAL" = "$CREDENCIAL_ESPERADA" ]; then
	relatar "credencial ${CREDENCIAL_FEDERADA} para ${SUJEITO_OIDC}" "já existia"
elif [ -n "$CREDENCIAL_ATUAL" ]; then
	executar_ou_planejar "credencial ${CREDENCIAL_FEDERADA} reconciliada para ${SUJEITO_OIDC}" "atualizado" \
		az ad app federated-credential update --id "$APP_ID" --federated-credential-id "$CREDENCIAL_FEDERADA" --parameters "$PARAMETROS_OIDC"
else
	executar_ou_planejar "credencial ${CREDENCIAL_FEDERADA} para ${SUJEITO_OIDC}" "criado" \
		az ad app federated-credential create --id "$APP_ID" --parameters "$PARAMETROS_OIDC"
fi

PASSO="4/5 identidade do job ${JOB} e AcrPull"
echo "[$PASSO]"
TIPO_IDENTIDADE=$(az containerapp job identity show -n "$JOB" -g "$GRUPO" --query type -o tsv 2>/dev/null || echo "None")
if [[ "$TIPO_IDENTIDADE" == *SystemAssigned* ]]; then
	relatar "identidade gerenciada do job" "já existia"
else
	executar_ou_planejar "identidade gerenciada do job (system-assigned)" "criado" \
		az containerapp job identity assign -n "$JOB" -g "$GRUPO" --system-assigned
fi
IDENTIDADE_DO_JOB=$(ler_identificador "identidade gerenciada do job" az containerapp job identity show -n "$JOB" -g "$GRUPO" --query principalId -o tsv)
atribuir_papel "papel AcrPull da identidade do job no registro" "$IDENTIDADE_DO_JOB" AcrPull "$ID_REGISTRO"

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
if [ "$(gh variable get ACR_NOME -R "$REPOSITORIO_GITHUB" 2>/dev/null || echo "")" = "$REGISTRO" ]; then
	relatar "variável ACR_NOME" "já existia"
else
	executar_ou_planejar "variável ACR_NOME" "gravado" gh variable set ACR_NOME -R "$REPOSITORIO_GITHUB" --body "$REGISTRO"
fi

SEGREDOS_EXISTENTES=$(gh secret list -R "$REPOSITORIO_GITHUB" --json name -q '.[].name')
gravar_segredo() {
	local nome="$1" valor="$2" existia=false
	if grep -qx "$nome" <<<"$SEGREDOS_EXISTENTES"; then
		existia=true
	fi
	if [ "$PLANO" = true ]; then
		if [ "$existia" = true ]; then
			relatar "segredo ${nome}" "já existia, seria regravado com o valor atual"
		else
			relatar "segredo ${nome}" "seria criado"
		fi
		return 0
	fi
	[ -n "$valor" ] || abortar "${nome} ficaria vazio; nada foi gravado"
	printf '%s' "$valor" | gh secret set "$nome" -R "$REPOSITORIO_GITHUB" >/dev/null
	if [ "$existia" = true ]; then
		relatar "segredo ${nome}" "já existia, regravado com o valor atual"
	else
		CRIADOS=$((CRIADOS + 1))
		relatar "segredo ${nome}" "criado"
	fi
}
gravar_segredo AZURE_CLIENT_ID "$APP_ID"
gravar_segredo AZURE_TENANT_ID "$TENANT"
gravar_segredo AZURE_SUBSCRIPTION_ID "$ASSINATURA"

PASSO="concluído"
if [ "$PLANO" = true ]; then
	echo "plano concluído, nada foi alterado"
else
	echo "concluído: ${CRIADOS} recurso(s) criado(s) ou alterado(s); segredos que já existiam foram regravados com o valor atual e não entram na conta"
fi
