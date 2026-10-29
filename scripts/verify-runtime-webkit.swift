import Cocoa
import WebKit

final class RuntimeProbe: NSObject, WKNavigationDelegate, WKScriptMessageHandler {
    let app = NSApplication.shared
    var window: NSWindow!
    var web: WKWebView!
    func start() throws {
        let fixture = try String(contentsOfFile: "scripts/desktop-design-fixture.mjs", encoding: .utf8)
            .replacingOccurrences(of: "export function", with: "function") + "\ninstallDesktopFixture();"
        let configuration = WKWebViewConfiguration()
        configuration.userContentController.addUserScript(WKUserScript(source: fixture, injectionTime: .atDocumentStart, forMainFrameOnly: true))
        configuration.userContentController.add(self, name: "verification")
        web = WKWebView(frame: NSRect(x: 0, y: 0, width: 1280, height: 620), configuration: configuration)
        web.navigationDelegate = self
        window = NSWindow(contentRect: web.frame, styleMask: [.titled], backing: .buffered, defer: false)
        window.contentView = web
        window.orderFront(nil)
        let origin = CommandLine.arguments.count > 1 ? CommandLine.arguments[1] : "http://127.0.0.1:4181/desktop.html"
        guard let url = URL(string: origin), url.host == "127.0.0.1" else { throw NSError(domain: "LocalServerRequired", code: 1) }
        web.load(URLRequest(url: url))
        DispatchQueue.main.asyncAfter(deadline: .now() + 55) { print("Runtime WebKit verification timeout"); exit(1) }
        app.run()
    }
    func webView(_ webView: WKWebView, didFinish navigation: WKNavigation!) {
        let script = """
        void (async () => {
          const wait = async predicate => {
            for (let n=0;n<200;n++) { if (predicate()) return; await new Promise(resolve=>setTimeout(resolve,20)); }
            throw Error('UI wait timed out');
          };
          const frame = () => new Promise(resolve=>requestAnimationFrame(resolve));
          await wait(()=>globalThis.__designShell);
          globalThis.__designOverrides['Platform.ListRuntimes'] = () => ({ok:true,value:{complete:true,runtimes:Array.from({length:12},(_,i)=>({runtimeId:i===0?'runtime-design':`runtime-${i}`,name:`Assistant ${i} with a long name to verify stable line wrapping`,status:'Ready',currentVersionNumber:1}))}});
          globalThis.__designNavigate('selectSettingsSection','Runtime');
          await wait(()=>document.querySelectorAll('.desktop-runtime').length===12);
          const results=[];
          for (const locale of ['en','zh-Hans']) {
            globalThis.__designLocale(locale);
            await frame(); await frame();
            for (const outcome of ['failure','success']) {
              const main=document.querySelector('.desktop-settings');
              const target=main.querySelectorAll('.desktop-runtime')[locale==='en'?6:7];
              const button=target.querySelector('button');
              target.scrollIntoView({block:'center'});
              await frame();
              const rect=element=>{const r=element.getBoundingClientRect();return [r.x,r.y,r.width,r.height];};
              const geometry=()=>[main.scrollTop,main.scrollHeight,main.clientWidth,...[...main.querySelectorAll('.desktop-runtime')].flatMap(row=>[...rect(row),...rect(row.firstElementChild),...rect(row.querySelector('button'))])];
              const before=geometry();
              let finish;
              globalThis.__designOverrides['Platform.BindRuntime']=(arg,original)=>new Promise(resolve=>{finish=()=>resolve(outcome==='success'?{ok:true,value:original(arg)}:{ok:false,error:{code:'testFailure',message:'Runtime binding failed. Please try again.'}});});
              const samples=[];
              let capture=true;
              const sample=()=>{if(capture){samples.push(geometry());requestAnimationFrame(sample);}};
              requestAnimationFrame(sample);
              button.click();
              await wait(()=>finish);
              if(target.dataset.selecting!=='true'||!button.disabled)throw Error('Missing pending binding state');
              await frame();await frame();
              finish();
              await wait(()=>target.dataset.selecting==='false');
              for(let i=0;i<12;i++)await frame();
              capture=false;
              samples.push(geometry());
              const delta=Math.max(...samples.flatMap(current=>current.map((value,index)=>Math.abs(value-before[index]))));
              if(delta>=0.5)throw Error(`WebKit Runtime layout moved ${delta}px (${locale}, ${outcome})`);
              if(target.dataset.selected!==String(outcome==='success'))throw Error('Binding selection state did not settle');
              results.push({locale,outcome,maxShift:delta});
            }
          }
          window.webkit.messageHandlers.verification.postMessage(JSON.stringify({pass:true,engine:'native WKWebView',input:'programmatic button click',results}));
        })().catch(error=>window.webkit.messageHandlers.verification.postMessage(JSON.stringify({pass:false,error:error.message})));
        """
        webView.evaluateJavaScript(script) { _, error in
            if let error { print(error.localizedDescription); exit(1) }
        }
    }
    func userContentController(_ userContentController: WKUserContentController, didReceive message: WKScriptMessage) {
        guard let result = message.body as? String else { print("Invalid verification result"); exit(1) }
        print(result)
        exit(result.contains("\"pass\":true") ? 0 : 1)
    }
}
try RuntimeProbe().start()
