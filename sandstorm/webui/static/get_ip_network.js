// This is adapted from:
//
// https://github.com/dwrensha/sandstorm-test-app/blob/ip-network/index.html

window.addEventListener("load", function(_event) {
	document.getElementById("request_cap").addEventListener("click", function() {
		var rpcId = Math.random();
		window.parent.postMessage({powerboxRequest: {
			rpcId: rpcId,
			query: ["EAZQAQEAABEBF1EEAQH_QCAqemtXgqkAAAA"], // ID for ipNetwork
		}}, "*");
		window.addEventListener("message", function(event) {
			if (event.data.rpcId !== rpcId) {
				return;
			}

			if (event.data.error) {
				console.error("rpc errored:", event.data.error);
				return;
			}

			var xhr = new XMLHttpRequest();
			xhr.open("POST", "/ip-network-cap", true);

			// Hack to pass bytes through unprocessed.
			xhr.overrideMimeType("text/plain; charset=x-user-defined");

			xhr.send(event.data.token);
		}, false);
	});
});
