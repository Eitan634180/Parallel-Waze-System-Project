@echo off

if "%~1"=="" (
  echo Env file path was not provided.
  exit /b 1
)

set "_LOAD_ENV_FILE=%~1"
if not exist "%_LOAD_ENV_FILE%" (
  echo Env file not found: %_LOAD_ENV_FILE%
  set "_LOAD_ENV_FILE="
  exit /b 1
)

for /f "usebackq eol=# tokens=1* delims==" %%A in ("%_LOAD_ENV_FILE%") do (
  call :set_value "%%~A" "%%~B"
)

set "_LOAD_ENV_FILE="
exit /b 0

:set_value
if "%~1"=="" exit /b 0
set "%~1=%~2"
exit /b 0
