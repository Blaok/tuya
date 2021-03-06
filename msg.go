// Copyright 2019 py60800.
// Use of this source code is governed by Apache-2 licence
// license that can be found in the LICENSE file.

// Tuya high level communication library

package tuya

import (
   "encoding/json"
   "fmt"
   "log"
   "time"
   "errors"
)

// create base messages
func (d *Appliance) makeBaseMsg() map[string]interface{} {
   d.mutex.RLock()
   defer d.mutex.RUnlock()
   m := make(map[string]interface{})
   m["devId"] = d.GwId
   m["uid"] = d.GwId
   m["t"] = time.Now().Unix()
   return m
}
func (d *Appliance) makeStatusMsg() map[string]interface{} {
   d.mutex.RLock()
   defer d.mutex.RUnlock()
   return map[string]interface{}{"gwId": d.GwId, "devId": d.GwId}
}
func (d *Appliance) initialStatusMsg() []byte {
   m := map[string]interface{}{"gwId": d.GwId, "devId": d.GwId}
   data, _ := json.Marshal(m)
   return data
}

// -------------------------------
func (d *Appliance) SendEncryptedCommand(cmd int, jdata interface{}) error {
   d.mutex.RLock()
   data, er1 := json.Marshal(jdata)
   if er1 != nil {
      d.mutex.RUnlock()
      return fmt.Errorf("Json Marshal (%v)", er1)
   }
   var b []byte
   switch d.Version {
      case "3.1":
         cipherText, er2 := aesEncrypt(data, d.key, true)
         if er2 != nil {
            d.mutex.RUnlock()
            return fmt.Errorf("Encrypt error(%v)", er2)
         }
         sig := md5Sign(cipherText, d.key, d.Version)
         b = make([]byte, 0, len(sig)+len(d.Version)+len(cipherText))
         b = append(b, []byte(d.Version)...)
         b = append(b, sig...)
         b = append(b, cipherText...)
      case "3.3":
         cipherText, er2 := aesEncrypt(data, d.key, false)
         if er2 != nil {
            d.mutex.RUnlock()
            return fmt.Errorf("Encrypt error(%v)", er2)
         }
         padding := "\000\000\000\000\000\000\000\000\000\000\000\000"
         b = make([]byte, 0, len(padding)+len(d.Version)+len(cipherText))
         b = append(b, []byte(d.Version)...)
         b = append(b, padding...)
         b = append(b, cipherText...)
      default:
         return errors.New("Unknown version")
   }
   d.mutex.RUnlock()

   select {
      case d.tcpChan <- query{cmd, b}:
      default:
         return errors.New("Device no ready")
   }
   return nil
}

func (d *Appliance) processResponse(code int, b []byte) {
   var i int
   for i = 0; i < len(b) && b[i] == byte(0); i++ {
   }
   b = b[i:]
   if len(b) == 0 { // can be an ack
      d.device.processResponse(code, b)
      return
   } // empty
   var data []byte
   if b[0] == byte('{') {
      //  Message in clear text
      data = b
   } else {
      encrypted := false
      if len(b) > (len(d.Version) + 16) {
         // Check if message starts with version number
         encrypted = true
         for i, vb := range d.Version {
            encrypted = encrypted && b[i] == byte(vb)
         }
      }
      if !encrypted {
         // can be an error message
         log.Println("Resp:", code, string(b))
         return
      }
      var err error
      switch d.Version {
      case "3.1":
         // ignore 16-byte signature
         data, err = aesDecrypt(b[len(d.Version)+16:], d.key, true)
      case "3.3":
         // https://github.com/codetheweb/tuyapi/blob/5a08481689c5062e17ff9a280d0e51815e2cafb7/lib/cipher.js#L54
         data, err = aesDecrypt(b[15:], d.key, false)
      default:
         log.Fatal("Unknown version")
      }
      if err != nil {
         log.Println("Decrypt error")
         return
      }
   }
   d.device.processResponse(code, data)
}

// Send message unencrypted
func (d *Appliance) SendCommand(cmd int, jdata interface{}) error {
   data, er1 := json.Marshal(jdata)
   if er1 != nil {
      return fmt.Errorf("Json Marshal(%v)", er1)
   }
   select {
      case d.tcpChan <- query{cmd, data}:
      default:
         return errors.New("Device no ready")
   }
   return nil
}
