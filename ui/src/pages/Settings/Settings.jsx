import React from 'react';
import {
  Breadcrumb,
  ControlBar,
  Input,
  Button,
  Block,
  BlockHeader,
  BlockBody,
  BlockFooter,
  InfoBar,
  InfoBox,
  Dropdown,
} from '@kerberos-io/ui';
// import { Link } from 'react-router-dom';
import './Settings.scss';
import timezones from './timezones';

// eslint-disable-next-line react/prefer-stateless-function
class Settings extends React.Component {
  KERBEROS_VAULT = 'kstorage'; // @TODO needs to change

  KERBEROS_HUB = 's3'; // @TODO needs to change

  constructor() {
    super();
    this.state = {
      search: '',
      global: {
        s3: {},
        kstorage: {},
      },
      mqttSuccess: false,
      mqttError: false,
      stunturnSuccess: false,
      stunturnError: false,
      generalSuccess: false,
      generalError: false,
      hubSuccess: false,
      hubError: false,
      verifyHubSuccess: false,
      verifyHubError: false,
      verifyHubErrorMessage: '',
      persistenceSuccess: false,
      persistenceError: false,
      verifyPersistenceSuccess: false,
      verifyPersistenceError: false,
      verifyPersistenceMessage: '',
      loading: false,
      loadingHub: false,
    };
    this.storageTypes = [
      {
        label: 'Kerberos Hub',
        value: this.KERBEROS_HUB,
      },
      {
        label: 'Kerberos Vault',
        value: this.KERBEROS_VAULT,
      },
    ];

    this.tags = {
      general: ['timezone', 'generic', 'general'],
      mqtt: ['mqtt', 'message broker', 'events', 'broker'],
      'stun-turn': ['webrtc', 'stun', 'turn', 'livestreaming'],
      'kerberos-hub': [
        'kerberos hub',
        'dashboard',
        'agents',
        'monitoring',
        'site',
      ],
      persistence: [
        'kerberos hub',
        'kerberos vault',
        'storage',
        'persistence',
        'gcp',
        'aws',
        'minio',
      ],
    };
    this.timezones = [];
    timezones.forEach((t) =>
      this.timezones.push({
        label: t.text,
        value: t.utc[0],
      })
    );

    this.changeValue = this.changeValue.bind(this);
    this.changeVaultValue = this.changeVaultValue.bind(this);
    this.changeS3Value = this.changeS3Value.bind(this);
    this.changeStorageType = this.changeStorageType.bind(this);
    this.changeTimezone = this.changeTimezone.bind(this);
    this.filterSettings = this.filterSettings.bind(this);
    this.saveGeneralSettings = this.saveGeneralSettings.bind(this);
    this.saveSTUNTURNSettings = this.saveSTUNTURNSettings.bind(this);
    this.saveMQTTSettings = this.saveMQTTSettings.bind(this);
    this.saveHubSettings = this.saveHubSettings.bind(this);
    this.savePersistenceSettings = this.savePersistenceSettings.bind(this);
    this.verifyPersistenceSettings = this.verifyPersistenceSettings.bind(this);
    this.verifyHubSettings = this.verifyHubSettings.bind(this);
  }

  changeValue() {
    console.log(this);
  }

  changeVaultValue() {
    console.log(this);
  }

  changeS3Value() {
    console.log(this);
  }

  changeStorageType() {
    console.log(this);
  }

  changeTimezone() {
    console.log(this);
  }

  filterSettings() {
    console.log(this);
  }

  saveGeneralSettings() {
    console.log(this);
  }

  saveSTUNTURNSettings() {
    console.log(this);
  }

  saveMQTTSettings() {
    console.log(this);
  }

  saveHubSettings() {
    console.log(this);
  }

  savePersistenceSettings() {
    console.log(this);
  }

  verifyPersistenceSettings() {
    console.log(this);
  }

  verifyHubSettings() {
    console.log(this);
  }

  render() {
    const {
      search,
      global,
      generalSuccess,
      generalError,
      mqttSuccess,
      mqttError,
      stunturnSuccess,
      stunturnError,
      hubSuccess,
      hubError,
      verifyHubSuccess,
      verifyHubError,
      verifyHubErrorMessage,
      persistenceSuccess,
      persistenceError,
      verifyPersistenceSuccess,
      verifyPersistenceError,
      verifyPersistenceMessage,
      loading,
      loadingHub,
    } = this.state;

    // Determine which section(s) to be shown, depending on the searching criteria.
    let showGeneralSection = true;
    let showMQTTSection = true;
    let showSTUNTURNSection = true;
    let showKerberosHubSection = true;
    let showPersistenceSection = true;

    if (search !== '' && search !== null) {
      if (this.tags) {
        const sections = Object.keys(this.tags);
        sections.forEach((section) => {
          // Find a match for the current section
          const sectionTags = this.tags[section];
          let match = false;
          sectionTags.forEach((tag) => {
            if (tag.toLowerCase().includes(search.toLowerCase())) {
              match = true;
            }
          });

          switch (section) {
            case 'general':
              showGeneralSection = match;
              break;
            case 'mqtt':
              showMQTTSection = match;
              break;
            case 'stun-turn':
              showSTUNTURNSection = match;
              break;
            case 'kerberos-hub':
              showKerberosHubSection = match;
              break;
            case 'persistence':
              showPersistenceSection = match;
              break;
            default:
          }
        });
      }
    }

    console.log(showGeneralSection);
    console.log(showPersistenceSection);
    console.log(showSTUNTURNSection);
    console.log(showMQTTSection);
    console.log(loading);
    console.log(verifyPersistenceMessage);
    console.log(verifyPersistenceError);
    console.log(verifyPersistenceSuccess);
    console.log(persistenceError);
    console.log(persistenceSuccess);
    console.log(verifyHubSuccess);
    console.log(stunturnError);
    console.log(stunturnSuccess);
    console.log(mqttError);
    console.log(mqttSuccess);
    console.log(generalError);
    console.log(generalSuccess);

    return (
      <div id="settings">
        <Breadcrumb
          title="Settings"
          level1="Onboard your camera"
          level1Link=""
        />

        <ControlBar>
          <Input
            iconleft="search"
            onChange={() => {}}
            placeholder="Search settings..."
            layout="controlbar"
            type="text"
          />
        </ControlBar>

        <div className="info">
          <InfoBox
            image="info-surveillance"
            title="Settings"
            description="Configure the Kerberos Agent as you wish. Specify the type of camera, region of interest, cloud storage and much more."
          />
        </div>

        <div className="stats grid-container --two-columns">
          <div>
            {showMQTTSection && (
              <Block>
                <BlockHeader>
                  <h4>MQTT</h4>
                </BlockHeader>
                <BlockBody>
                  {mqttSuccess && (
                    <InfoBar
                      type="success"
                      message="MQTT settings are successfully saved."
                    />
                  )}
                  {mqttError && (
                    <InfoBar
                      type="alert"
                      message="Something went wrong while saving."
                    />
                  )}
                  <p>
                    A MQTT broker is used to communicate from{' '}
                    <a
                      href="https://doc.kerberos.io/hub/first-things-first/"
                      rel="noopener noreferrer"
                      target="_blank"
                    >
                      Kerberos Hub
                    </a>{' '}
                    to the Kerberos Agent, to achieve for example livestreaming
                    or ONVIF (PTZ) capabilities.
                  </p>
                  <Input
                    label="Broker Uri"
                    value={global.mqtturi}
                    onChange={(value) => this.changeValue('mqtturi', value)}
                  />
                  <Input
                    label="Username"
                    value={global.mqtt_username}
                    onChange={(value) =>
                      this.changeValue('mqtt_username', value)
                    }
                  />
                  <Input
                    label="Password"
                    value={global.mqtt_password}
                    onChange={(value) =>
                      this.changeValue('mqtt_password', value)
                    }
                  />
                </BlockBody>
                <BlockFooter>
                  <Button
                    label="Save"
                    onClick={this.saveMQTTSettings}
                    type="default"
                    icon="pencil"
                  />
                </BlockFooter>
              </Block>
            )}

            {/* STUN/TURN block */}
            {showSTUNTURNSection && (
              <Block>
                <BlockHeader>
                  <h4>STUN/TURN for WebRTC</h4>
                </BlockHeader>
                <BlockBody>
                  {stunturnSuccess && (
                    <InfoBar
                      type="success"
                      message="STUN/TURN settings are successfully saved."
                    />
                  )}
                  {stunturnError && (
                    <InfoBar
                      type="alert"
                      message="Something went wrong while saving."
                    />
                  )}
                  <p>
                    For full-resolution livestreaming we use the concept of
                    WebRTC. One of the key capabilities is the ICE-candidate
                    feature, which allows NAT traversal using the concepts of
                    STUN/TURN.
                  </p>
                  <Input
                    label="STUN server"
                    value={global.stunuri}
                    onChange={(value) => this.changeValue('stunuri', value)}
                  />
                  <Input
                    label="TURN server"
                    value={global.turnuri}
                    onChange={(value) => this.changeValue('turnuri', value)}
                  />
                  <Input
                    label="Username"
                    value={global.turn_username}
                    onChange={(value) =>
                      this.changeValue('turn_username', value)
                    }
                  />
                  <Input
                    label="Password"
                    value={global.turn_password}
                    onChange={(value) =>
                      this.changeValue('turn_password', value)
                    }
                  />
                </BlockBody>
                <BlockFooter>
                  <Button
                    label="Save"
                    onClick={this.saveSTUNTURNSettings}
                    type="default"
                    icon="pencil"
                  />
                </BlockFooter>
              </Block>
            )}
          </div>

          <div>
            {showKerberosHubSection && (
              <Block>
                <BlockHeader>
                  <h4>Kerberos Hub</h4>
                </BlockHeader>
                <BlockBody>
                  {false && (
                    <InfoBar
                      type="loading"
                      message="Verifying your Kerberos Hub settings."
                    />
                  )}
                  {false && (
                    <InfoBar
                      type="success"
                      message="Kerberos Hub settings are successfully verified."
                    />
                  )}
                  {verifyHubError && (
                    <InfoBar type="alert" message={verifyHubErrorMessage} />
                  )}
                  {hubSuccess && (
                    <InfoBar
                      type="success"
                      message="Kerberos Hub settings are successfully saved."
                    />
                  )}
                  {hubError && (
                    <InfoBar
                      type="alert"
                      message="Something went wrong while saving."
                    />
                  )}
                  <p>
                    Kerberos Agents can send heartbeats to a central{' '}
                    <a
                      href="https://doc.kerberos.io/hub/first-things-first/"
                      rel="noopener noreferrer"
                      target="_blank"
                    >
                      Kerberos Hub
                    </a>{' '}
                    installation. Heartbeats and other relevant information are
                    synced to Kerberos Hub to show realtime information about
                    your video landscape.
                  </p>
                  <Input
                    label="API url"
                    placeholder="The API for Kerberos Hub."
                    value={global.hub_uri}
                    onChange={(value) => this.changeValue('hub_uri', value)}
                  />
                  <Input
                    label="Public key"
                    placeholder="The public key granted to your Kerberos Hub account."
                    value={global.hub_key}
                    onChange={(value) => this.changeValue('hub_key', value)}
                  />
                  <Input
                    label="Private key"
                    placeholder="The private key granted to your Kerberos Hub account."
                    value={global.hub_private_key}
                    onChange={(value) =>
                      this.changeValue('hub_private_key', value)
                    }
                  />
                  <Input
                    label="Site"
                    value={global.hub_site}
                    placeholder="The site ID the Kerberos Agents are belonging to in Kerberos Hub."
                    onChange={(value) => this.changeValue('hub_site', value)}
                  />
                </BlockBody>
                <BlockFooter>
                  <Button
                    label="Verify Connection"
                    disabled={loadingHub}
                    type={loadingHub ? 'neutral' : 'default'}
                    onClick={this.verifyHubSettings}
                    icon="verify"
                  />
                  <Button
                    label="Save"
                    onClick={this.saveHubSettings}
                    type="default"
                    icon="pencil"
                  />
                </BlockFooter>
              </Block>
            )}

            {/* Persistence block */}
            {showPersistenceSection && (
              <Block>
                <BlockHeader>
                  <h4>Persistence</h4>
                </BlockHeader>
                <BlockBody>
                  {loading && (
                    <InfoBar
                      type="loading"
                      message="Verifying your persistence settings."
                    />
                  )}
                  {verifyPersistenceSuccess && (
                    <InfoBar
                      type="success"
                      message="Persistence settings are successfully verified."
                    />
                  )}
                  {verifyPersistenceError && (
                    <InfoBar type="alert" message={verifyPersistenceMessage} />
                  )}
                  {persistenceSuccess && (
                    <InfoBar
                      type="success"
                      message="Persistence settings are successfully saved."
                    />
                  )}
                  {persistenceError && (
                    <InfoBar
                      type="alert"
                      message="Something went wrong while saving your settings."
                    />
                  )}
                  <p>
                    Having the ability to store your recordings is the beginning
                    of everything. You can choose between our{' '}
                    <a
                      href="https://doc.kerberos.io/hub/license/"
                      rel="noopener noreferrer"
                      target="_blank"
                    >
                      Kerberos Hub (SAAS offering)
                    </a>{' '}
                    or your own{' '}
                    <a
                      href="https://doc.kerberos.io/vault/first-things-first/"
                      rel="noopener noreferrer"
                      target="_blank"
                    >
                      Kerberos Vault
                    </a>{' '}
                    deployment.
                  </p>
                  <Dropdown
                    isRadio
                    placeholder="Select a persistence"
                    selected={[global.cloud]}
                    items={this.storageTypes}
                    onChange={this.changeStorageType}
                  />

                  {global.cloud === this.KERBEROS_HUB && (
                    <>
                      <Input
                        label="Kerberos Hub API URL"
                        placeholder="The API endpoint for uploading your recordings."
                        value={global.s3.proxyuri}
                        onChange={(value) =>
                          this.changeS3Value('proxyuri', value)
                        }
                      />
                      <Input
                        label="Region"
                        placeholder="The region we are storing our recordings in."
                        value={global.s3.region}
                        onChange={(value) =>
                          this.changeS3Value('region', value)
                        }
                      />
                      <Input
                        label="Bucket"
                        placeholder="The bucket we are storing our recordings in."
                        value={global.s3.bucket}
                        onChange={(value) =>
                          this.changeS3Value('bucket', value)
                        }
                      />
                      <Input
                        label="Username/Directory"
                        placeholder="The username of your Kerberos Hub account."
                        value={global.s3.username}
                        onChange={(value) =>
                          this.changeS3Value('username', value)
                        }
                      />
                    </>
                  )}

                  {global.cloud === this.KERBEROS_VAULT && (
                    <>
                      <Input
                        label="Kerberos Vault API URL"
                        placeholder="The Kerberos Vault API"
                        value={global.kstorage.uri}
                        onChange={(value) =>
                          this.changeVaultValue('uri', value)
                        }
                      />
                      <Input
                        label="Provider"
                        placeholder="The provider to which your recordings will be send."
                        value={global.kstorage.provider}
                        onChange={(value) =>
                          this.changeVaultValue('provider', value)
                        }
                      />
                      <Input
                        label="Directory"
                        placeholder="Sub directory the recordings will be stored in your provider."
                        value={global.kstorage.directory}
                        onChange={(value) =>
                          this.changeVaultValue('directory', value)
                        }
                      />
                      <Input
                        label="Access key"
                        placeholder="The access key of your Kerberos Vault account."
                        value={global.kstorage.access_key}
                        onChange={(value) =>
                          this.changeVaultValue('access_key', value)
                        }
                      />
                      <Input
                        label="Secret key"
                        placeholder="The secret key of your Kerberos Vault account."
                        value={global.kstorage.secret_access_key}
                        onChange={(value) =>
                          this.changeVaultValue('secret_access_key', value)
                        }
                      />
                    </>
                  )}
                </BlockBody>
                <BlockFooter>
                  <Button
                    label="Verify Connection"
                    disabled={loading}
                    onClick={this.verifyPersistenceSettings}
                    type={loading ? 'neutral' : 'default'}
                    icon="verify"
                  />
                  <Button
                    label="Save"
                    type="submit"
                    onClick={this.savePersistenceSettings}
                    buttonType="submit"
                    icon="pencil"
                  />
                </BlockFooter>
              </Block>
            )}
          </div>
        </div>
      </div>
    );
  }
}
export default Settings;
