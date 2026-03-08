grut.toast("Hello", "Extension loaded!", "info")

grut.register_command("hello", "Say hello", function()
  grut.toast("Hello", "Hello from Lua!", "info")
end)
