@echo off
setlocal

if "%KIRO_NSSM%"=="" set "KIRO_NSSM=C:\Users\thaild\Downloads\nssm-2.24\win64\nssm.exe"
if "%KIRO_SERVICE%"=="" set "KIRO_SERVICE=kiro-go"
if "%KIRO_SITE%"=="" set "KIRO_SITE=apiunlimit.escs.vn"
if "%KIRO_PORT%"=="" set "KIRO_PORT=9180"
if "%KIRO_SITE_DIR%"=="" set "KIRO_SITE_DIR=F:\website\apiunlimit.escs.vn"
if "%KIRO_LOG_DIR%"=="" set "KIRO_LOG_DIR=%KIRO_SITE_DIR%\logs"
if "%KIRO_DATA_DIR%"=="" set "KIRO_DATA_DIR=%KIRO_SITE_DIR%\data"
if "%KIRO_BIN%"=="" set "KIRO_BIN=%KIRO_SITE_DIR%\kiro-go.exe"

if "%~1"=="" goto help

set "COMMAND=%~1"

if /I "%COMMAND%"=="start" goto start
if /I "%COMMAND%"=="stop" goto stop
if /I "%COMMAND%"=="restart" goto restart
if /I "%COMMAND%"=="status" goto status
if /I "%COMMAND%"=="install" goto install
if /I "%COMMAND%"=="uninstall" goto uninstall
if /I "%COMMAND%"=="test" goto test
if /I "%COMMAND%"=="local" goto local
if /I "%COMMAND%"=="site" goto site
if /I "%COMMAND%"=="admin" goto admin
if /I "%COMMAND%"=="port" goto port
if /I "%COMMAND%"=="logs" goto logs
if /I "%COMMAND%"=="iis" goto iis
if /I "%COMMAND%"=="config" goto config
if /I "%COMMAND%"=="help" goto help

echo Unknown command: %COMMAND%
echo.
goto help

:start
call :require_nssm
if errorlevel 1 exit /b 1
"%KIRO_NSSM%" start "%KIRO_SERVICE%"
exit /b

:stop
call :require_nssm
if errorlevel 1 exit /b 1
"%KIRO_NSSM%" stop "%KIRO_SERVICE%"
exit /b

:restart
call :require_nssm
if errorlevel 1 exit /b 1
"%KIRO_NSSM%" restart "%KIRO_SERVICE%"
exit /b

:status
call :require_nssm
if errorlevel 1 exit /b 1
"%KIRO_NSSM%" status "%KIRO_SERVICE%"
exit /b

:install
call :require_nssm
if errorlevel 1 exit /b 1
if not exist "%KIRO_BIN%" (
  echo Binary not found: %KIRO_BIN%
  exit /b 1
)
if "%KIRO_ADMIN_PASSWORD%"=="" (
  echo KIRO_ADMIN_PASSWORD env var not set. Refusing to install with default password.
  echo Run: set KIRO_ADMIN_PASSWORD=your_strong_password
  exit /b 1
)
if not exist "%KIRO_DATA_DIR%" mkdir "%KIRO_DATA_DIR%"
if not exist "%KIRO_LOG_DIR%" mkdir "%KIRO_LOG_DIR%"
"%KIRO_NSSM%" status "%KIRO_SERVICE%" >nul 2>&1
if not errorlevel 1 (
  echo Service already exists. Stopping and removing first...
  "%KIRO_NSSM%" stop "%KIRO_SERVICE%" >nul 2>&1
  "%KIRO_NSSM%" remove "%KIRO_SERVICE%" confirm >nul 2>&1
)
"%KIRO_NSSM%" install "%KIRO_SERVICE%" "%KIRO_BIN%"
"%KIRO_NSSM%" set "%KIRO_SERVICE%" AppDirectory "%KIRO_SITE_DIR%"
"%KIRO_NSSM%" set "%KIRO_SERVICE%" AppEnvironmentExtra "CONFIG_PATH=%KIRO_DATA_DIR%\config.json" "ADMIN_PASSWORD=%KIRO_ADMIN_PASSWORD%" "LOG_LEVEL=info"
"%KIRO_NSSM%" set "%KIRO_SERVICE%" AppStdout "%KIRO_LOG_DIR%\stdout.log"
"%KIRO_NSSM%" set "%KIRO_SERVICE%" AppStderr "%KIRO_LOG_DIR%\stderr.log"
"%KIRO_NSSM%" set "%KIRO_SERVICE%" AppRotateFiles 1
"%KIRO_NSSM%" set "%KIRO_SERVICE%" AppRotateOnline 1
"%KIRO_NSSM%" set "%KIRO_SERVICE%" AppRotateBytes 10485760
"%KIRO_NSSM%" set "%KIRO_SERVICE%" Start SERVICE_AUTO_START
"%KIRO_NSSM%" set "%KIRO_SERVICE%" AppThrottle 5000
"%KIRO_NSSM%" set "%KIRO_SERVICE%" AppRestartDelay 5000
"%KIRO_NSSM%" set "%KIRO_SERVICE%" Description "Kiro-Go reverse proxy service"
"%KIRO_NSSM%" start "%KIRO_SERVICE%"
exit /b

:uninstall
call :require_nssm
if errorlevel 1 exit /b 1
"%KIRO_NSSM%" stop "%KIRO_SERVICE%" >nul 2>&1
"%KIRO_NSSM%" remove "%KIRO_SERVICE%" confirm
exit /b

:test
curl.exe -I "http://127.0.0.1:%KIRO_PORT%/admin"
echo.
curl.exe -I "https://%KIRO_SITE%/admin"
exit /b

:local
curl.exe -I "http://127.0.0.1:%KIRO_PORT%/admin"
exit /b

:site
curl.exe -I "https://%KIRO_SITE%/admin"
exit /b

:admin
start "" "https://%KIRO_SITE%/admin"
exit /b

:port
netstat -ano | findstr ":%KIRO_PORT%"
exit /b

:logs
if not exist "%KIRO_LOG_DIR%" (
  echo Log folder not found: %KIRO_LOG_DIR%
  exit /b 1
)
if exist "%KIRO_LOG_DIR%\stderr.log" (
  echo ===== stderr.log =====
  type "%KIRO_LOG_DIR%\stderr.log"
  echo.
)
if exist "%KIRO_LOG_DIR%\stdout.log" (
  echo ===== stdout.log =====
  type "%KIRO_LOG_DIR%\stdout.log"
  echo.
)
exit /b

:iis
%windir%\system32\inetsrv\appcmd.exe stop site "%KIRO_SITE%"
%windir%\system32\inetsrv\appcmd.exe start site "%KIRO_SITE%"
exit /b

:config
echo NSSM:        %KIRO_NSSM%
echo Service:     %KIRO_SERVICE%
echo Site:        %KIRO_SITE%
echo Port:        %KIRO_PORT%
echo Site dir:    %KIRO_SITE_DIR%
echo Data dir:    %KIRO_DATA_DIR%
echo Log dir:     %KIRO_LOG_DIR%
echo Binary:      %KIRO_BIN%
exit /b

:require_nssm
if exist "%KIRO_NSSM%" exit /b 0
echo NSSM not found: %KIRO_NSSM%
echo Edit kiro.cmd or set KIRO_NSSM to the correct nssm.exe path.
exit /b 1

:help
echo Usage: kiro start^|stop^|restart^|status^|install^|uninstall^|test^|local^|site^|admin^|port^|logs^|iis^|config
echo.
echo Commands:
echo   kiro install    Install NSSM service (requires KIRO_ADMIN_PASSWORD env var)
echo   kiro uninstall  Remove NSSM service
echo   kiro status     Show NSSM service status
echo   kiro start      Start Kiro-Go service
echo   kiro stop       Stop Kiro-Go service
echo   kiro restart    Restart Kiro-Go service
echo   kiro test       Test local backend and public IIS URL
echo   kiro local      Test http://127.0.0.1:%KIRO_PORT%/admin
echo   kiro site       Test https://%KIRO_SITE%/admin
echo   kiro admin      Open admin panel in browser
echo   kiro port       Show process using port %KIRO_PORT%
echo   kiro logs       Print NSSM stdout/stderr logs
echo   kiro iis        Restart IIS site only
echo   kiro config     Show current wrapper config
exit /b 0
