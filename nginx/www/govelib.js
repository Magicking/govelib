var endpoint = "/map/";
var map = L.map('map').setView([48.8582, 2.3400], 13);

var tiles = L.tileLayer('http://{s}.tile.osm.org/{z}/{x}/{y}.png', {
    attribution: '&copy; <a href="http://osm.org/copyright">OpenStreetMap</a> contributors',
}).addTo(map);
zoom = getParameterByName('zoom');
if (zoom == '')
  zoom = 17;
var heat = L.heatLayer([],{maxZoom:zoom,}).addTo(map);

function getParameterByName(name, url) {
  if (!url) {
    url = window.location.href;
  }   
  name = name.replace(/[\[\]]/g, "\\$&");
  var regex = new RegExp("[?&]" + name + "(=([^&#]*)|&|#|$)"),
      results = regex.exec(url);
  if (!results) return null;
  if (!results[2]) return ''; 
  return decodeURIComponent(results[2].replace(/\+/g, " "));
}

startMonitoring(getParameterByName('kind'));
function startMonitoring(kind)
{
  var url = endpoint + kind;
  console.log("Monitoring: " + url);
  $.ajax({url:url,})
    .done(function( data ) {
      heat.setLatLngs(data);
    }
  );
  setInterval(function(){
    $.ajax({url:url,})
      .done(function( data ) {
        heat.setLatLngs(data);
      }
    );
  }, 60 * 1000);
}
