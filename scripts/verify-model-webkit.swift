import Cocoa
import WebKit

final class Probe: NSObject, WKNavigationDelegate, WKScriptMessageHandler {
    let app = NSApplication.shared
    var window: NSWindow!
    var web: WKWebView!
    func start() {
        let fixture = try! String(contentsOfFile: "scripts/desktop-design-fixture.mjs", encoding: .utf8).replacingOccurrences(of: "export function", with: "function") + "\ninstallDesktopFixture();"
        let config = WKWebViewConfiguration()
        config.userContentController.addUserScript(WKUserScript(source: fixture, injectionTime: .atDocumentStart, forMainFrameOnly: true))
        config.userContentController.add(self, name: "verification")
        web = WKWebView(frame: NSRect(x: 0, y: 0, width: 1280, height: 900), configuration: config)
        web.navigationDelegate = self
        window = NSWindow(contentRect: web.frame, styleMask: [.titled], backing: .buffered, defer: false)
        window.contentView = web
        web.load(URLRequest(url: URL(string: "http://127.0.0.1:4174/desktop.html")!))
        DispatchQueue.main.asyncAfter(deadline: .now() + 45) { print("WebKit verification timeout"); exit(1) }
        app.run()
    }
    func webView(_ webView: WKWebView, didFinish navigation: WKNavigation!) {
        let script = """
        void (async () => {
          const wait = async (predicate) => { for(let n=0;n<200;n++){ const value=predicate();if(value)return value;await new Promise(r=>setTimeout(r,50)); }throw Error('UI wait timed out'); };
          const pick = () => document.querySelector('.assistant-dialog');
          await wait(()=>globalThis.__designShell?.selectedSession);
          globalThis.__designNavigate('newConversation');
          const editor=await wait(pick);
          const input=editor.querySelector('input');
          Object.getOwnPropertyDescriptor(HTMLInputElement.prototype,'value').set.call(input,'WebKit model regression');input.dispatchEvent(new Event('input',{bubbles:true}));
          editor.querySelector('.composer-model-trigger').click();
          const option=await wait(()=>[...editor.querySelectorAll('[role=radio]')].find(n=>n.textContent.trim()==='design-alternate'));
          option.dispatchEvent(new MouseEvent('mousedown',{bubbles:true,cancelable:true}));option.click();
          await wait(()=>editor.querySelector('.composer-model-trigger').textContent.includes('design-alternate'));
          editor.querySelector('form').dispatchEvent(new Event('submit',{bubbles:true,cancelable:true}));
          await wait(()=>!pick());
          await wait(()=>document.querySelector('.composer-model-trigger')?.textContent.includes('design-alternate'));
          const update=globalThis.__designRequests.find(n=>n.method==='Session.UpdateExecutionConfig');
          if(update?.arg.model!=='design-alternate')throw Error('Model was not persisted');
          window.webkit.messageHandlers.verification.postMessage(JSON.stringify({pass:true,engine:'native WKWebView',model:update.arg.model,composer:document.querySelector('.composer-model-trigger').textContent}));
        })().catch(error=>window.webkit.messageHandlers.verification.postMessage(JSON.stringify({pass:false,error:error.message})));
        """
        webView.evaluateJavaScript(script) { _, error in if let error { print(error.localizedDescription); exit(1) } }
    }
    func userContentController(_ userContentController: WKUserContentController, didReceive message: WKScriptMessage) {
        let result = message.body as? String ?? "Invalid result"
        print(result)
        exit(result.contains("\"pass\":true") ? 0 : 1)
    }
}
let probe = Probe()
probe.start()
