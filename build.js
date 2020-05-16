const fs = require('fs');
const path = require('path');

exports.prebuild = async function prebuild() {
    if (!fs.existsSync(path.resolve(__dirname, 'file'))) fs.mkdirSync(path.resolve(__dirname, 'file'));
    fs.copyFileSync(
        path.resolve(__dirname, 'executorserver'),
        path.resolve(__dirname, 'file', 'executorserver'),
    );
    fs.copyFileSync(
        path.resolve(__dirname, 'executorserver.exe'),
        path.resolve(__dirname, 'file', 'executorserver.exe'),
    );
};
