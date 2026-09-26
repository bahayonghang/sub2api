#Requires -Version 5.1
<#
.SYNOPSIS
    按《本地部署指南》重新部署本机 Sub2API。

.DESCRIPTION
    默认动作会构建 sub2api:local，并只重建应用容器。
    运行目录是仓库内的 local-deploy。Compose 文件使用 deploy\docker-compose.local.yml。
    宿主机端口保持 8081。已有 .env、data、postgres_data、redis_data 保持原文件。
    数据目录缺少 config.yaml 或 .installed 时直接停止，不执行首次安装。

.PARAMETER Action
    Deploy  构建镜像并重建应用容器。默认值。
    Restart 不构建，重启应用容器。
    Up      不构建，按现有 Compose 启动三个服务。
    Down    停止三个服务并保留本地数据目录。
    Status  查看容器、健康检查和数据库行数。

.EXAMPLE
    powershell -NoProfile -ExecutionPolicy Bypass -File .\redeploy-local.ps1
#>
[CmdletBinding()]
param(
    [ValidateSet('Deploy', 'Restart', 'Up', 'Down', 'Status')]
    [string]$Action = 'Deploy'
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
if (Get-Variable -Name PSNativeCommandUseErrorActionPreference -ErrorAction SilentlyContinue) {
    $PSNativeCommandUseErrorActionPreference = $false
}

$SourceRoot = Split-Path -Parent $MyInvocation.MyCommand.Path
$DeployDir = Join-Path $SourceRoot 'local-deploy'
$ComposeFile = Join-Path $SourceRoot 'deploy\docker-compose.local.yml'
$ImageName = 'sub2api:local'
$HostPort = 8081
$ContainerPort = 8080
$DockerDesktop = 'C:\Program Files\Docker\Docker\Docker Desktop.exe'

function Invoke-Native {
    param(
        [Parameter(Mandatory = $true)]
        [scriptblock]$Command
    )
    & $Command
    if ($LASTEXITCODE -ne 0) {
        throw "命令失败，退出码 $LASTEXITCODE"
    }
}

function Get-EnvValues {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Path,
        [Parameter(Mandatory = $true)]
        [string]$Key
    )
    $values = @()
    foreach ($line in [System.IO.File]::ReadAllLines($Path)) {
        if ($line -match "^\s*$([regex]::Escape($Key))=(.*)$") {
            $value = $Matches[1].Trim()
            if ($value.Length -ge 2 -and $value.StartsWith('"') -and $value.EndsWith('"')) {
                $value = $value.Substring(1, $value.Length - 2)
            }
            $values += $value
        }
    }
    return $values
}

function Assert-ExistingDeployment {
    $required = @(
        (Join-Path $DeployDir '.env'),
        $ComposeFile,
        (Join-Path $DeployDir 'data\config.yaml'),
        (Join-Path $DeployDir 'data\.installed'),
        (Join-Path $DeployDir 'postgres_data'),
        (Join-Path $DeployDir 'redis_data'),
        (Join-Path $SourceRoot 'Dockerfile')
    )
    foreach ($path in $required) {
        if (-not (Test-Path -LiteralPath $path)) {
            throw "缺少已有部署文件：$path"
        }
    }

    $envFile = Join-Path $DeployDir '.env'
    $ports = @(Get-EnvValues -Path $envFile -Key 'SERVER_PORT')
    if ($ports.Count -eq 0 -or @($ports | Where-Object { $_ -ne "$HostPort" }).Count -gt 0) {
        throw ".env 中的 SERVER_PORT 必须全部为 $HostPort"
    }

    foreach ($key in @('POSTGRES_PASSWORD', 'JWT_SECRET', 'TOTP_ENCRYPTION_KEY', 'ADMIN_PASSWORD', 'ADMIN_EMAIL')) {
        $values = @(Get-EnvValues -Path $envFile -Key $key | Where-Object { -not [string]::IsNullOrWhiteSpace($_) })
        if ($values.Count -eq 0) {
            throw ".env 缺少非空的 $key。脚本不会生成新值。"
        }
    }
}

function Assert-PortAvailable {
    $published = @(docker ps --filter "publish=$HostPort" --format '{{.Names}}')
    if ($LASTEXITCODE -ne 0) {
        throw '无法查询 Docker 端口映射'
    }
    $foreign = @($published | Where-Object { $_ -and $_ -ne 'sub2api' })
    if ($foreign.Count -gt 0) {
        throw "端口 $HostPort 已由其它容器占用：$($foreign -join ', ')"
    }

    $listeners = @()
    try {
        $listeners = @(Get-NetTCPConnection -LocalPort $HostPort -State Listen -ErrorAction SilentlyContinue)
    }
    catch {
        $listeners = @()
    }
    if ($listeners.Count -eq 0 -or $published -contains 'sub2api') {
        return
    }

    $dockerProcesses = @('com.docker.backend', 'docker-proxy', 'wslrelay', 'vpnkit', 'com.docker.proxy')
    foreach ($procId in @($listeners | Select-Object -ExpandProperty OwningProcess -Unique)) {
        $process = Get-Process -Id $procId -ErrorAction SilentlyContinue
        if ($process -and $dockerProcesses -notcontains $process.ProcessName) {
            throw "端口 $HostPort 被进程 $($process.ProcessName) ($procId) 占用"
        }
    }
}

function Wait-Docker {
    Invoke-Native { docker info --format '{{.ServerVersion}}' } | Out-Null
}

function Start-DockerDesktop {
    try {
        Wait-Docker
        return
    }
    catch {
        if (-not (Test-Path -LiteralPath $DockerDesktop)) {
            throw "Docker 引擎未就绪，且未找到 $DockerDesktop"
        }
        Write-Host 'Docker 引擎未就绪，正在启动 Docker Desktop'
        Start-Process -FilePath $DockerDesktop | Out-Null
    }

    for ($i = 0; $i -lt 60; $i++) {
        Start-Sleep -Seconds 5
        try {
            Wait-Docker
            Write-Host 'Docker 引擎已就绪'
            return
        }
        catch {
        }
    }
    throw '等待 Docker 引擎超时'
}

function Ensure-ImageOverride {
    $path = Join-Path $DeployDir 'docker-compose.local-image.yml'
    $content = @"
# 只替换应用镜像。账号、端口和数据目录仍由 docker-compose.local.yml 与 .env 决定。
services:
  sub2api:
    image: $ImageName
"@
    $utf8 = New-Object System.Text.UTF8Encoding $false
    [System.IO.File]::WriteAllText($path, $content.Replace("`r`n", "`n"), $utf8)
}

function Invoke-Compose {
    param(
        [Parameter(Mandatory = $true)]
        [string[]]$ComposeArgs
    )
    $overrideFile = Join-Path $DeployDir 'docker-compose.local-image.yml'
    Invoke-Native {
        docker compose --project-directory $DeployDir -f $ComposeFile -f $overrideFile @ComposeArgs
    }
}

function Wait-Healthy {
    for ($i = 0; $i -lt 24; $i++) {
        $state = docker inspect sub2api --format '{{.State.Status}} {{if .State.Health}}{{.State.Health.Status}}{{end}}'
        if ($LASTEXITCODE -ne 0) {
            throw '无法读取 sub2api 容器状态'
        }
        Write-Host "sub2api $state"
        if ($state -eq 'running healthy') {
            return
        }
        if ($state -match '^(exited|dead)') {
            throw "sub2api 已停止：$state"
        }
        Start-Sleep -Seconds 5
    }
    throw '等待 sub2api 健康检查超时'
}

function Assert-RedeployResult {
    $image = docker inspect sub2api --format '{{.Config.Image}}'
    if ($LASTEXITCODE -ne 0 -or $image -ne $ImageName) {
        throw "运行镜像为 '$image'，要求 $ImageName"
    }

    $ports = docker inspect sub2api --format '{{json .NetworkSettings.Ports}}'
    if ($ports -notmatch ('"HostPort"\s*:\s*"{0}"' -f $HostPort)) {
        throw "宿主机端口不是 ${HostPort}：$ports"
    }

    $health = "$((curl.exe -fsS "http://127.0.0.1:${HostPort}/health") )".Trim()
    if ($LASTEXITCODE -ne 0 -or $health -ne '{"status":"ok"}') {
        throw "健康检查失败：$health"
    }

    $logs = @(docker logs sub2api 2>&1 | ForEach-Object { "$_" }) -join "`n"
    if ($logs -match 'Admin user created') {
        throw '启动日志出现 Admin user created。已有数据目录可能未挂载到 ./data'
    }

    $envFile = Join-Path $DeployDir '.env'
    $pgUser = [string]@(Get-EnvValues -Path $envFile -Key 'POSTGRES_USER' | Where-Object { -not [string]::IsNullOrWhiteSpace($_) } | Select-Object -Last 1)
    $pgDb = [string]@(Get-EnvValues -Path $envFile -Key 'POSTGRES_DB' | Where-Object { -not [string]::IsNullOrWhiteSpace($_) } | Select-Object -Last 1)
    if ([string]::IsNullOrWhiteSpace($pgUser)) { $pgUser = 'sub2api' }
    if ([string]::IsNullOrWhiteSpace($pgDb)) { $pgDb = 'sub2api' }

    $counts = docker exec sub2api-postgres psql -U $pgUser -d $pgDb -tAc "SELECT 'users=' || COUNT(*) FROM users; SELECT 'accounts=' || COUNT(*) FROM accounts; SELECT 'api_keys=' || COUNT(*) FROM api_keys;"
    if ($LASTEXITCODE -ne 0) {
        throw '无法读取数据库行数'
    }
    if ($counts -match 'users=0') {
        throw 'users 表行数为 0'
    }

    $rootMarker = [System.IO.Path]::GetFileName($DeployDir)
    foreach ($name in @('sub2api', 'sub2api-postgres', 'sub2api-redis')) {
        $sources = docker inspect $name --format '{{range .Mounts}}{{.Source}}{{println}}{{end}}'
        if ($LASTEXITCODE -ne 0) {
            throw "无法读取 $name 的挂载"
        }
        $mounted = @($sources -split "`n" | ForEach-Object { $_.Trim() } | Where-Object { $_ })
        if ($mounted.Count -eq 0) {
            throw "$name 没有挂载"
        }
        $inside = $false
        foreach ($source in $mounted) {
            if ($source -match 'sub2api_2\.8\.14') {
                throw "$name 仍挂载旧发布目录：$source"
            }
            if ($source -match [regex]::Escape($rootMarker)) {
                $inside = $true
            }
        }
        if (-not $inside) {
            throw "$name 没有挂载运行目录"
        }
    }

    $lan = @(Get-NetIPAddress -AddressFamily IPv4 -ErrorAction SilentlyContinue |
        Where-Object {
            $_.IPAddress -notlike '127.*' -and
            $_.IPAddress -notlike '198.18.*' -and
            $_.IPAddress -notlike '198.19.*' -and
            $_.PrefixOrigin -ne 'WellKnown' -and
            $_.AddressState -eq 'Preferred' -and
            $_.InterfaceAlias -notmatch 'vEthernet|WSL|Hyper-V|Docker|Virtual|VMware|VirtualBox|Loopback|Meta|Clash|sing-box'
        } |
        Select-Object -ExpandProperty IPAddress -Unique)

    Write-Host "镜像 $image"
    Write-Host "健康检查 $health"
    Write-Host "后台 http://localhost:$HostPort"
    foreach ($address in $lan) {
        Write-Host "局域网 http://${address}:$HostPort"
    }
    foreach ($line in @($counts -split "`n" | Where-Object { -not [string]::IsNullOrWhiteSpace($_) })) {
        Write-Host $line.Trim()
    }
}

$envFile = Join-Path $DeployDir '.env'
$envStamp = $null
if (Test-Path -LiteralPath $envFile) {
    $envStamp = (Get-Item -LiteralPath $envFile).LastWriteTimeUtc
}

Start-DockerDesktop
Assert-ExistingDeployment
Assert-PortAvailable
Ensure-ImageOverride

switch ($Action) {
    'Deploy' {
        Write-Host "构建 $ImageName"
        Invoke-Native { docker build -t $ImageName -f (Join-Path $SourceRoot 'Dockerfile') $SourceRoot }
        Invoke-Compose -ComposeArgs @('up', '-d', '--pull', 'never', 'postgres', 'redis')
        Invoke-Compose -ComposeArgs @('up', '-d', '--pull', 'never', '--no-deps', '--force-recreate', 'sub2api')
        Wait-Healthy
        Assert-RedeployResult
    }
    'Restart' {
        Invoke-Compose -ComposeArgs @('restart', 'sub2api')
        Wait-Healthy
        Assert-RedeployResult
    }
    'Up' {
        Invoke-Compose -ComposeArgs @('up', '-d', '--pull', 'never')
        Wait-Healthy
        Assert-RedeployResult
    }
    'Down' {
        Invoke-Compose -ComposeArgs @('down')
        Write-Host '三个服务已停止。data、postgres_data、redis_data 仍在运行目录。'
    }
    'Status' {
        Invoke-Compose -ComposeArgs @('ps')
        try {
            Wait-Healthy
            Assert-RedeployResult
        }
        catch {
            Write-Host $_.Exception.Message
            exit 1
        }
    }
}

if ($envStamp -and (Test-Path -LiteralPath $envFile)) {
    $envNow = (Get-Item -LiteralPath $envFile).LastWriteTimeUtc
    if ($envNow -ne $envStamp) {
        throw '.env 的修改时间发生了变化'
    }
}

Write-Host "完成：$Action"
