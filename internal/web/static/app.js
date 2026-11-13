// Small progressive enhancements. Everything works without JavaScript.
(function () {
  // Copy buttons on the "Post the job" page.
  document.querySelectorAll("[data-copy]").forEach(function (btn) {
    btn.addEventListener("click", function () {
      var el = document.getElementById(btn.getAttribute("data-copy"));
      if (!el) return;
      el.select();
      var done = function () { var t = btn.textContent; btn.textContent = "Copied"; setTimeout(function () { btn.textContent = t; }, 1500); };
      if (navigator.clipboard) { navigator.clipboard.writeText(el.value).then(done, function () { document.execCommand("copy"); done(); }); }
      else { document.execCommand("copy"); done(); }
    });
  });

  // Select-all checkbox on the ranked list.
  var all = document.querySelector("[data-select-all]");
  if (all) {
    all.addEventListener("change", function () {
      document.querySelectorAll("input[name=ids]").forEach(function (cb) { cb.checked = all.checked; });
    });
  }

  function anySelected() { return document.querySelector("input[name=ids]:checked") !== null; }
  function typing() { var a = document.activeElement; return a && (a.tagName === "INPUT" || a.tagName === "TEXTAREA" || a.tagName === "SELECT"); }

  // While screening runs, poll the job's status and reload when scores land.
  var status = document.getElementById("screening-status");
  if (status) {
    var url = status.getAttribute("data-status-url");
    var pending = parseInt(status.getAttribute("data-pending"), 10);
    var timer = setInterval(function () {
      fetch(url, { credentials: "same-origin" }).then(function (r) { return r.json(); }).then(function (d) {
        if (d.pending !== pending && !anySelected() && !typing()) { clearInterval(timer); location.reload(); }
      }).catch(function () {});
    }, 8000);
  }

  // Pages that should refresh themselves (import progress, waiting for screening).
  var auto = document.querySelector("[data-autorefresh]");
  if (auto) {
    var secs = parseInt(auto.getAttribute("data-autorefresh"), 10) || 10;
    setTimeout(function () { if (!typing()) location.reload(); }, secs * 1000);
  }
})();
