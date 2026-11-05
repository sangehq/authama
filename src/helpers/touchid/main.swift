// authama-touchid asks macOS for Touch ID on authama's behalf. authama is a
// Go binary built without cgo, so it cannot call LocalAuthentication itself;
// it runs this helper and reads its exit status. Touch ID stands in for the
// owner PIN at the terminal; any failure falls back to the PIN.
//
//   authama-touchid --check            0 when Touch ID can be asked for
//   authama-touchid "unlock authama"   asks; 0 when the finger matched
//
// Exit status: 0 ok, 1 failed, 2 no Touch ID here (none, no password set,
// or not interactive), 3 no finger enrolled, 4 cancelled, 5 locked out after
// too many tries, 64 usage. Biometrics only: a password fallback would add
// nothing over authama's own PIN.
import Foundation
import LocalAuthentication

let args = Array(CommandLine.arguments.dropFirst())
if args.contains("--help") || args.contains("-h") {
    print("usage: authama-touchid [--check] [reason]")
    exit(0)
}
if args.filter({ $0.hasPrefix("-") && $0 != "--check" }).count > 0 {
    FileHandle.standardError.write("usage: authama-touchid [--check] [reason]\n".data(using: .utf8)!)
    exit(64)
}

func status(_ error: Error?) -> Int32 {
    guard let e = error as? LAError else { return 1 }
    switch e.code {
    case .biometryNotAvailable, .passcodeNotSet, .notInteractive: return 2
    case .biometryNotEnrolled: return 3
    case .userCancel, .userFallback, .systemCancel, .appCancel: return 4
    case .biometryLockout: return 5
    default: return 1
    }
}

let context = LAContext()
context.localizedFallbackTitle = ""  // no "Enter Password" button: the PIN is the fallback
let policy = LAPolicy.deviceOwnerAuthenticationWithBiometrics
var error: NSError?
guard context.canEvaluatePolicy(policy, error: &error) else { exit(status(error)) }
if args.contains("--check") { exit(0) }

let reason = args.first(where: { !$0.hasPrefix("-") }) ?? "unlock authama"
let done = DispatchSemaphore(value: 0)
var code: Int32 = 1
context.evaluatePolicy(policy, localizedReason: reason) { ok, err in
    code = ok ? 0 : status(err)
    done.signal()
}
done.wait()
exit(code)
