/**
 * Cross-browser clipboard helper that works in both Secure Contexts (HTTPS/localhost)
 * and Non-Secure Contexts (HTTP on local LAN IP like http://192.168.1.7).
 */
export async function copyToClipboard(text: string): Promise<boolean> {
  if (!text) return false

  // 1. Try modern navigator.clipboard API if in a secure context
  if (window.isSecureContext && navigator?.clipboard?.writeText) {
    try {
      await navigator.clipboard.writeText(text)
      return true
    } catch {
      // Fall through to fallback
    }
  }

  // 2. Reliable fallback using document.execCommand('copy') with temporary textarea
  try {
    const textarea = document.createElement('textarea')
    textarea.value = text
    textarea.setAttribute('readonly', '')
    textarea.style.position = 'fixed'
    textarea.style.left = '-9999px'
    textarea.style.top = '-9999px'
    textarea.style.opacity = '0'
    document.body.appendChild(textarea)

    // Selection for both desktop & mobile
    textarea.focus()
    textarea.select()
    textarea.setSelectionRange(0, textarea.value.length)

    const successful = document.execCommand('copy')
    document.body.removeChild(textarea)
    if (successful) return true
  } catch {
    // Fall through to prompt
  }

  // 3. Fallback to window.prompt if execCommand is blocked
  try {
    window.prompt('Nhấn Ctrl+C để sao chép:', text)
    return true
  } catch {
    return false
  }
}
