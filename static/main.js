function debounce(func, timeout = 2000) {
    let timer;
    return (...args) => {
        clearTimeout(timer);
        timer = setTimeout(() => { func.apply(this, args); }, timeout);
    };
}

function saveInput() {
    var element = document.getElementById("destination");
    var req = new XMLHttpRequest();
    req.open("POST", "/apiz/save", true);
    req.send(JSON.stringify({
        source: window.location.pathname.replace("/edit/", ""),
        destination: element.value,
    }));
}

window.processChange = debounce(() => saveInput());

window.editItem = function(item) {
    window.open(window.location.protocol + "//" + window.location.host + "/edit/" + encodeURI(item), "_blank")
}

window.deleteItem = function(element, item) {
    element.parentNode.parentNode.remove();
    var req = new XMLHttpRequest();
    req.open("POST", "/apiz/save", true);
    req.send(JSON.stringify({
        source: item,
        destination: "",
    }));
}