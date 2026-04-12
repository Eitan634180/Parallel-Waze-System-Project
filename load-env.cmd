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
  call :set_if_missing "%%~A" "%%~B"
)

set "_LOAD_ENV_FILE="
exit /b 0

:set_if_missing
if "%~1"=="" exit /b 0
call set "_LOAD_ENV_CURRENT=%%%~1%%"
if not defined _LOAD_ENV_CURRENT set "%~1=%~2"
set "_LOAD_ENV_CURRENT="
exit /b 0
