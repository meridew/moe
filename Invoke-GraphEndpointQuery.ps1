param(
    [Parameter(Mandatory)]
    [string]$Endpoint,
    
    [ValidateSet('v1.0', 'beta')]
    [string]$ApiVersion = 'beta',
    
    [string]$AppId,
    [string]$TenantId,
    [string]$Secret
)

$useParallel = $AppId -and $TenantId -and $Secret

if ($useParallel)
{
    $secureSecret = ConvertTo-SecureString $Secret -AsPlainText -Force
    $credential = [PSCredential]::new($AppId, $secureSecret)
    Connect-MgGraph -ClientSecretCredential $credential -TenantId $TenantId -NoWelcome
}
else
{
    Connect-MgGraph -NoWelcome
}

# Fetch schema and discover endpoints
$meta = [xml](Invoke-MgGraphRequest -Uri "https://graph.microsoft.com/$ApiVersion/`$metadata" -OutputType HttpResponseMessage).Content.ReadAsStringAsync().Result
$schema = $meta.edmx.DataServices.Schema | Where-Object Namespace -eq 'microsoft.graph'
$singleton = $schema.EntityContainer.Singleton | Where-Object Name -eq $Endpoint

if (-not $singleton) { throw "Endpoint '$Endpoint' not found" }

$typeName = $singleton.Type -replace '^microsoft\.graph\.', ''
$navProps = ($schema.EntityType | Where-Object Name -eq $typeName).NavigationProperty

$uris = @($navProps | 
    Where-Object { $_.Attributes['Type'].Value -match '^Collection' } |
    ForEach-Object { @{ Name = $_.Name; Uri = "/$ApiVersion/$Endpoint/$($_.Name)" } })

# Fetch data scriptblock
$fetch = {
    param($Uri, $AppId, $TenantId, $Secret)
    
    if ($AppId)
    {
        Import-Module Microsoft.Graph.Authentication -Force
        $cred = [PSCredential]::new($AppId, (ConvertTo-SecureString $Secret -AsPlainText -Force))
        Connect-MgGraph -ClientSecretCredential $cred -TenantId $TenantId -NoWelcome
    }
    
    $data = @()

    try
    {
        do
        {
            $r = Invoke-MgGraphRequest -Uri $Uri -OutputType PSObject -ErrorAction Stop
            if ($r.value) { $data += [pscustomobject]$r.value }
            $nextLink = $r.PSObject.Properties['@odata.nextLink']
            $Uri = if ($nextLink) { $nextLink.Value } else { $null }
        } while ($Uri)
    }
    catch
    {
        $data = "ERROR: $($_.Exception.Message)"
    }

    $data
}

$result = @{}
$total = $uris.Count

if ($useParallel)
{
    # Start jobs with progress
    $jobs = for ($i = 0; $i -lt $total; $i++)
    {
        Write-Progress -Activity "Starting jobs" -Status "$i of $total" -PercentComplete (($i / $total) * 100)
        Start-Job -Name $uris[$i].Name -ScriptBlock $fetch -ArgumentList $uris[$i].Uri, $AppId, $TenantId, $Secret
    }
    Write-Progress -Activity "Starting jobs" -Completed
    
    # Monitor jobs
    while ($jobs.State -match 'Running')
    {
        $running = @($jobs | Where-Object State -eq 'Running').Count
        $done = @($jobs | Where-Object State -eq 'Completed').Count
        Write-Progress -Activity "Fetching $Endpoint" -Status "Running: $running | Done: $done of $total"
        Start-Sleep -Milliseconds 500
    }
    Write-Progress -Activity "Fetching $Endpoint" -Completed
    
    # Collect results
    foreach ($job in $jobs)
    {
        $result[$job.Name] = @(Receive-Job $job)
        Remove-Job $job
    }
}
else
{
    # Sequential with progress
    for ($i = 0; $i -lt $total; $i++)
    {
        $u = $uris[$i]
        Write-Progress -Activity "Fetching $Endpoint" -Status "$($u.Name)" -PercentComplete (($i / $total) * 100)
        
        $data = @(& $fetch $u.Uri)
        $result[$u.Name] = $data
    }
    Write-Progress -Activity "Fetching $Endpoint" -Completed
}

Get-Job | Stop-Job -passThru | Remove-Job -Force

return $result